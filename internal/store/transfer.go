package store

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vancanhuit/devopsbin/internal/store/sqlc"
)

// serializationFailureCode and deadlockDetectedCode are the PostgreSQL
// SQLSTATEs for transient transaction conflicts. Both are safe to retry by
// replaying the whole transaction.
const (
	serializationFailureCode = "40001"
	deadlockDetectedCode     = "40P01"
)

// defaultTransferMaxRetries bounds how many times a serialization/deadlock
// conflict is retried before giving up with ErrRetriesExhausted.
const defaultTransferMaxRetries = 5

// Backoff bounds for retrying serialization/deadlock conflicts. Each retry
// waits a randomized duration in [0, min(cap, base*2^attempt)) so that
// contending transactions de-synchronize ("decorrelated jitter") instead of
// colliding again in lock-step and exhausting the retry budget.
const (
	transferRetryBaseBackoff = 1 * time.Millisecond
	transferRetryMaxBackoff  = 50 * time.Millisecond
)

// retryBackoff returns a randomized backoff duration for the given 1-based
// attempt number, capped at transferRetryMaxBackoff.
func retryBackoff(attempt int) time.Duration {
	maxDelay := transferRetryBaseBackoff << (attempt - 1)
	if maxDelay <= 0 || maxDelay > transferRetryMaxBackoff {
		maxDelay = transferRetryMaxBackoff
	}
	return time.Duration(rand.Int64N(int64(maxDelay)))
}

// Transfer-related sentinel errors. Callers (HTTP handlers) map these to status
// codes: invalid -> 400, not owner -> 403, not found -> 404, insufficient and
// retries exhausted -> 409.
var (
	// ErrInvalidTransfer is returned when the request is structurally invalid,
	// such as a non-positive amount or identical source and destination.
	ErrInvalidTransfer = errors.New("store: invalid transfer")
	// ErrAccountNotFound is returned when the source or destination account
	// does not exist.
	ErrAccountNotFound = errors.New("store: account not found")
	// ErrNotAccountOwner is returned when the session user does not own the
	// source account.
	ErrNotAccountOwner = errors.New("store: not account owner")
	// ErrInsufficientFunds is returned when the source balance cannot cover the
	// transfer amount.
	ErrInsufficientFunds = errors.New("store: insufficient funds")
	// ErrRetriesExhausted is returned when repeated serialization conflicts
	// prevent the transfer from committing within the retry budget.
	ErrRetriesExhausted = errors.New("store: serialization retries exhausted")
)

// TransferParams describes an authenticated money transfer between two
// accounts. SessionUserID is the id of the authenticated caller, who must own
// the source account.
type TransferParams struct {
	SessionUserID string
	FromAccountID string
	ToAccountID   string
	AmountCents   int64
	// Isolation selects the transaction isolation level: "serializable"
	// (default), "repeatable-read", or "read-committed". An empty or unknown
	// value defaults to serializable.
	Isolation string
	// HoldMs optionally widens the contention window by sleeping inside the
	// transaction after locking the accounts, to demonstrate isolation and
	// serialization retries under concurrency. Zero disables the delay.
	HoldMs int
	// MaxRetries optionally overrides the serialization retry budget. Zero uses
	// defaultTransferMaxRetries.
	MaxRetries int
}

// TransferResult reports the committed transfer and the resulting balances.
type TransferResult struct {
	TransferID       string
	FromAccountID    string
	ToAccountID      string
	FromBalanceCents int64
	ToBalanceCents   int64
	AmountCents      int64
	CreatedAt        time.Time
	// Attempts is the number of transaction attempts made, including retries,
	// so the demo can surface serialization contention.
	Attempts int
}

// isolationLevel maps a request string to a pgx isolation level, defaulting to
// Serializable for the strongest guarantees.
func isolationLevel(name string) pgx.TxIsoLevel {
	switch name {
	case "repeatable-read":
		return pgx.RepeatableRead
	case "read-committed":
		return pgx.ReadCommitted
	default:
		return pgx.Serializable
	}
}

// isRetryableConflict reports whether err is a transient serialization or
// deadlock failure that can be resolved by replaying the transaction.
func isRetryableConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == serializationFailureCode || pgErr.Code == deadlockDetectedCode
}

// Transfer moves AmountCents from the source to the destination account inside a
// single transaction, enforcing that the caller owns the source account and has
// sufficient funds. It runs at the configured isolation level (serializable by
// default) and retries the whole transaction on serialization or deadlock
// conflicts up to the retry budget, surfacing ErrRetriesExhausted if exceeded.
func (s *Store) Transfer(ctx context.Context, params TransferParams) (TransferResult, error) {
	if params.AmountCents <= 0 {
		return TransferResult{}, ErrInvalidTransfer
	}
	if params.FromAccountID == params.ToAccountID {
		return TransferResult{}, ErrInvalidTransfer
	}

	fromID, err := parseUUID(params.FromAccountID)
	if err != nil {
		return TransferResult{}, ErrAccountNotFound
	}
	toID, err := parseUUID(params.ToAccountID)
	if err != nil {
		return TransferResult{}, ErrAccountNotFound
	}
	ownerID, err := parseUUID(params.SessionUserID)
	if err != nil {
		return TransferResult{}, ErrNotAccountOwner
	}

	maxRetries := params.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultTransferMaxRetries
	}
	isoLevel := isolationLevel(params.Isolation)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := s.transferOnce(ctx, isoLevel, params, fromID, toID, ownerID)
		if err == nil {
			result.Attempts = attempt
			return result, nil
		}
		// Non-retryable domain or infrastructure errors fail immediately.
		if !isRetryableConflict(err) {
			return TransferResult{}, err
		}
		lastErr = err
		// Back off with jitter before replaying, unless this was the last
		// attempt. Abort early if the caller's context is cancelled.
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return TransferResult{}, fmt.Errorf("%w: %w", ctx.Err(), lastErr)
			case <-time.After(retryBackoff(attempt)):
			}
		}
	}
	return TransferResult{}, fmt.Errorf("%w: %d attempts: %v", ErrRetriesExhausted, maxRetries, lastErr)
}

// transferOnce executes a single attempt of the transfer transaction. A
// retryable conflict is returned unwrapped so the caller can detect and replay
// it; domain failures are returned as sentinel errors.
func (s *Store) transferOnce(
	ctx context.Context,
	isoLevel pgx.TxIsoLevel,
	params TransferParams,
	fromID, toID, ownerID pgtype.UUID,
) (TransferResult, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: isoLevel})
	if err != nil {
		return TransferResult{}, fmt.Errorf("store: begin transfer tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := sqlc.New(tx)

	// Lock both accounts FOR UPDATE in a deterministic id order to avoid
	// deadlocks between concurrent transfers touching the same pair.
	locked, err := q.GetAccountsForUpdate(ctx, []pgtype.UUID{fromID, toID})
	if err != nil {
		return TransferResult{}, fmt.Errorf("store: lock accounts: %w", err)
	}

	balances := make(map[string]int64, len(locked))
	owners := make(map[string]string, len(locked))
	for _, row := range locked {
		id := uuidString(row.ID)
		balances[id] = row.BalanceCents
		owners[id] = uuidString(row.UserID)
	}

	fromBalance, fromExists := balances[params.FromAccountID]
	if !fromExists {
		return TransferResult{}, ErrAccountNotFound
	}
	if _, toExists := balances[params.ToAccountID]; !toExists {
		return TransferResult{}, ErrAccountNotFound
	}
	if owners[params.FromAccountID] != params.SessionUserID {
		return TransferResult{}, ErrNotAccountOwner
	}
	if fromBalance < params.AmountCents {
		return TransferResult{}, ErrInsufficientFunds
	}

	// Optionally widen the contention window to demonstrate isolation and
	// serialization retries under concurrency.
	if params.HoldMs > 0 {
		seconds := float64(params.HoldMs) / 1000.0
		if _, err := tx.Exec(ctx, "SELECT pg_sleep($1)", seconds); err != nil {
			return TransferResult{}, fmt.Errorf("store: hold transfer: %w", err)
		}
	}

	newFrom, err := q.AdjustBalance(ctx, sqlc.AdjustBalanceParams{ID: fromID, DeltaCents: -params.AmountCents})
	if err != nil {
		return TransferResult{}, fmt.Errorf("store: debit source: %w", err)
	}
	newTo, err := q.AdjustBalance(ctx, sqlc.AdjustBalanceParams{ID: toID, DeltaCents: params.AmountCents})
	if err != nil {
		return TransferResult{}, fmt.Errorf("store: credit destination: %w", err)
	}

	ledger, err := q.InsertTransfer(ctx, sqlc.InsertTransferParams{
		FromAccountID: fromID,
		ToAccountID:   toID,
		AmountCents:   params.AmountCents,
	})
	if err != nil {
		return TransferResult{}, fmt.Errorf("store: record transfer: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		// A serialization failure typically surfaces at commit; return it
		// unwrapped so the retry loop can detect it.
		if isRetryableConflict(err) {
			return TransferResult{}, err
		}
		return TransferResult{}, fmt.Errorf("store: commit transfer: %w", err)
	}

	return TransferResult{
		TransferID:       uuidString(ledger.ID),
		FromAccountID:    params.FromAccountID,
		ToAccountID:      params.ToAccountID,
		FromBalanceCents: newFrom,
		ToBalanceCents:   newTo,
		AmountCents:      params.AmountCents,
		CreatedAt:        ledger.CreatedAt.Time,
	}, nil
}
