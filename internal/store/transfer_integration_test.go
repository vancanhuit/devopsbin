//go:build integration

// Integration tests for the authenticated account-transfer flow. Run via:
//
//	mise run api:test:integration
//
// (which brings up the test-profile infra and sets -tags=integration).

package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vancanhuit/devopsbin/internal/store"
	"github.com/vancanhuit/devopsbin/internal/store/sqlc"
)

// seedTransferAccount creates a fresh user with a single account at the given
// balance and returns their canonical user and account ids. Using a dedicated
// user per test keeps balances independent of the shared seed data and of other
// tests running against the same database.
func seedTransferAccount(t *testing.T, s *store.Store, balanceCents int64) (userID, accountID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var uid, aid pgtype.UUID
	err := s.WithTx(ctx, func(q *sqlc.Queries) error {
		u, err := q.CreateUser(ctx, sqlc.CreateUserParams{
			Username:     uniqueUsername(t),
			PasswordHash: "hash",
			Role:         "user",
		})
		if err != nil {
			return err
		}
		uid = u.ID
		a, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{
			UserID:       u.ID,
			Name:         "Checking",
			BalanceCents: balanceCents,
		})
		if err != nil {
			return err
		}
		aid = a.ID
		return nil
	})
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}
	return uuid.UUID(uid.Bytes).String(), uuid.UUID(aid.Bytes).String()
}

// accountBalance reads an account's current balance by id for assertions.
func accountBalance(t *testing.T, s *store.Store, accountID string) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var pgID pgtype.UUID
	if err := pgID.Scan(accountID); err != nil {
		t.Fatalf("scan account id: %v", err)
	}
	acct, err := s.Queries().GetAccountByID(ctx, pgID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	return acct.BalanceCents
}

// TestStore_Transfer_HappyPath moves funds between two accounts and verifies the
// balances, the returned result, and that a ledger row was written.
func TestStore_Transfer_HappyPath(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	aliceID, fromID := seedTransferAccount(t, s, 100000)
	_, toID := seedTransferAccount(t, s, 50000)

	got, err := s.Transfer(ctx, store.TransferParams{
		SessionUserID: aliceID,
		FromAccountID: fromID,
		ToAccountID:   toID,
		AmountCents:   2500,
	})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if got.FromBalanceCents != 97500 || got.ToBalanceCents != 52500 {
		t.Fatalf("result balances = (%d, %d), want (97500, 52500)", got.FromBalanceCents, got.ToBalanceCents)
	}
	if got.AmountCents != 2500 {
		t.Fatalf("amount = %d, want 2500", got.AmountCents)
	}
	if got.Attempts < 1 {
		t.Fatalf("attempts = %d, want >= 1", got.Attempts)
	}
	if _, err := uuid.Parse(got.TransferID); err != nil {
		t.Fatalf("transfer id %q is not a valid uuid: %v", got.TransferID, err)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("transfer result has a zero created_at")
	}

	if bal := accountBalance(t, s, fromID); bal != 97500 {
		t.Fatalf("source balance = %d, want 97500", bal)
	}
	if bal := accountBalance(t, s, toID); bal != 52500 {
		t.Fatalf("destination balance = %d, want 52500", bal)
	}

	// A ledger row must exist for the transfer.
	transfers, err := s.ListTransfers(ctx)
	if err != nil {
		t.Fatalf("ListTransfers: %v", err)
	}
	found := false
	for _, tr := range transfers {
		if tr.ID == got.TransferID {
			found = true
			if tr.AmountCents != 2500 {
				t.Fatalf("ledger amount = %d, want 2500", tr.AmountCents)
			}
			break
		}
	}
	if !found {
		t.Fatalf("transfer %q not found in ledger", got.TransferID)
	}
}

// TestStore_Transfer_InsufficientFunds rejects a transfer larger than the source
// balance and leaves both balances untouched.
func TestStore_Transfer_InsufficientFunds(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	aliceID, fromID := seedTransferAccount(t, s, 1000)
	_, toID := seedTransferAccount(t, s, 0)

	_, err := s.Transfer(ctx, store.TransferParams{
		SessionUserID: aliceID,
		FromAccountID: fromID,
		ToAccountID:   toID,
		AmountCents:   5000,
	})
	if !errors.Is(err, store.ErrInsufficientFunds) {
		t.Fatalf("err = %v, want ErrInsufficientFunds", err)
	}
	if bal := accountBalance(t, s, fromID); bal != 1000 {
		t.Fatalf("source balance changed to %d, want 1000", bal)
	}
	if bal := accountBalance(t, s, toID); bal != 0 {
		t.Fatalf("destination balance changed to %d, want 0", bal)
	}
}

// TestStore_Transfer_NotOwner rejects a transfer when the session user does not
// own the source account.
func TestStore_Transfer_NotOwner(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// The source account belongs to "owner"; "attacker" tries to move its funds.
	_, fromID := seedTransferAccount(t, s, 100000)
	attackerID, toID := seedTransferAccount(t, s, 0)

	_, err := s.Transfer(ctx, store.TransferParams{
		SessionUserID: attackerID,
		FromAccountID: fromID,
		ToAccountID:   toID,
		AmountCents:   1000,
	})
	if !errors.Is(err, store.ErrNotAccountOwner) {
		t.Fatalf("err = %v, want ErrNotAccountOwner", err)
	}
	if bal := accountBalance(t, s, fromID); bal != 100000 {
		t.Fatalf("source balance changed to %d, want 100000", bal)
	}
}

// TestStore_Transfer_AccountNotFound maps a missing source or destination
// account to ErrAccountNotFound.
func TestStore_Transfer_AccountNotFound(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	aliceID, fromID := seedTransferAccount(t, s, 100000)
	const missingID = "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4cff"

	_, err := s.Transfer(ctx, store.TransferParams{
		SessionUserID: aliceID,
		FromAccountID: fromID,
		ToAccountID:   missingID,
		AmountCents:   1000,
	})
	if !errors.Is(err, store.ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound", err)
	}
}

// TestStore_Transfer_InvalidAmount rejects non-positive amounts and identical
// source and destination accounts.
func TestStore_Transfer_InvalidAmount(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	aliceID, fromID := seedTransferAccount(t, s, 100000)
	_, toID := seedTransferAccount(t, s, 0)

	if _, err := s.Transfer(ctx, store.TransferParams{
		SessionUserID: aliceID,
		FromAccountID: fromID,
		ToAccountID:   toID,
		AmountCents:   0,
	}); !errors.Is(err, store.ErrInvalidTransfer) {
		t.Fatalf("zero amount err = %v, want ErrInvalidTransfer", err)
	}

	if _, err := s.Transfer(ctx, store.TransferParams{
		SessionUserID: aliceID,
		FromAccountID: fromID,
		ToAccountID:   fromID,
		AmountCents:   1000,
	}); !errors.Is(err, store.ErrInvalidTransfer) {
		t.Fatalf("same-account err = %v, want ErrInvalidTransfer", err)
	}
}

// TestStore_Transfer_ConcurrentConserves runs many bidirectional transfers
// between two accounts concurrently. The deterministic lock ordering avoids
// deadlocks and the serialization-retry loop resolves conflicts, so every
// transfer must commit and the total balance must be conserved.
func TestStore_Transfer_ConcurrentConserves(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	const (
		startBalance = 100000
		perDirection = 8
		amount       = 1000
	)

	aliceID, accountA := seedTransferAccount(t, s, startBalance)
	bobID, accountB := seedTransferAccount(t, s, startBalance)

	var wg sync.WaitGroup
	errs := make(chan error, perDirection*2)

	transfer := func(sessionID, from, to string) {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// A small in-transaction hold widens the contention window so the
		// isolation and retry behaviour is genuinely exercised. This stress
		// scenario (many writers on the same two rows, each holding locks)
		// generates far more serialization conflicts than normal load, so it
		// grants a larger retry budget than the API's default of five; the
		// backoff loop must still let every transfer eventually commit.
		_, err := s.Transfer(ctx, store.TransferParams{
			SessionUserID: sessionID,
			FromAccountID: from,
			ToAccountID:   to,
			AmountCents:   amount,
			HoldMs:        5,
			MaxRetries:    25,
		})
		errs <- err
	}

	for range perDirection {
		wg.Add(2)
		go transfer(aliceID, accountA, accountB) // A -> B, owned by alice
		go transfer(bobID, accountB, accountA)   // B -> A, owned by bob
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent transfer failed: %v", err)
		}
	}

	// Equal numbers of equal-sized transfers in each direction net to zero, so
	// both balances must return to the starting amount and the total is
	// conserved.
	balA := accountBalance(t, s, accountA)
	balB := accountBalance(t, s, accountB)
	if balA+balB != startBalance*2 {
		t.Fatalf("total balance = %d, want %d", balA+balB, startBalance*2)
	}
	if balA != startBalance || balB != startBalance {
		t.Fatalf("balances = (%d, %d), want (%d, %d)", balA, balB, startBalance, startBalance)
	}
}
