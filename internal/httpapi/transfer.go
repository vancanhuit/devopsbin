package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/vancanhuit/devopsbin/internal/store"
)

// GetAccounts lists every account so a transfer source and destination can be
// chosen. The session check is enforced by the auth middleware before this
// runs; any authenticated user may call it.
func (s *Server) GetAccounts(ctx context.Context, _ GetAccountsRequestObject) (GetAccountsResponseObject, error) {
	accounts, err := s.users.ListAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminAccount, 0, len(accounts))
	for _, a := range accounts {
		id, err := uuid.Parse(a.ID)
		if err != nil {
			return nil, fmt.Errorf("httpapi: parse account id %q: %w", a.ID, err)
		}
		out = append(out, AdminAccount{
			Id:            id,
			OwnerUsername: a.OwnerUsername,
			Name:          a.Name,
			BalanceCents:  a.BalanceCents,
			CreatedAt:     a.CreatedAt,
		})
	}
	return GetAccounts200JSONResponse{Accounts: out}, nil
}

// PostTransfer moves funds from the caller's source account to a destination
// account inside a single transaction. The session and CSRF checks are enforced
// by the auth middleware before this runs; this handler enforces ownership of
// the source account and sufficient funds, mapping store errors to the matching
// status codes.
func (s *Server) PostTransfer(ctx context.Context, request PostTransferRequestObject) (PostTransferResponseObject, error) {
	if request.Body == nil {
		return PostTransfer400JSONResponse{Error: "request body is required"}, nil
	}

	sess, ok := sessionFrom(ctx)
	if !ok {
		// The middleware guarantees a session for this op; treat its absence as
		// an authentication failure rather than panicking.
		return PostTransfer401JSONResponse{Error: "authentication required"}, nil
	}

	params := store.TransferParams{
		SessionUserID: sess.UserID,
		FromAccountID: request.Body.FromAccountId.String(),
		ToAccountID:   request.Body.ToAccountId.String(),
		AmountCents:   request.Body.AmountCents,
	}
	if request.Params.Isolation != nil {
		params.Isolation = string(*request.Params.Isolation)
	}
	if request.Params.HoldMs != nil {
		params.HoldMs = int(*request.Params.HoldMs)
	}

	result, err := s.users.Transfer(ctx, params)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidTransfer):
			return PostTransfer400JSONResponse{Error: "invalid transfer: amount must be positive and accounts must differ"}, nil
		case errors.Is(err, store.ErrNotAccountOwner):
			return PostTransfer403JSONResponse{Error: "you do not own the source account"}, nil
		case errors.Is(err, store.ErrAccountNotFound):
			return PostTransfer404JSONResponse{Error: "source or destination account not found"}, nil
		case errors.Is(err, store.ErrInsufficientFunds):
			return PostTransfer409JSONResponse{Error: "insufficient funds"}, nil
		case errors.Is(err, store.ErrRetriesExhausted):
			return PostTransfer409JSONResponse{Error: "transfer could not commit due to serialization conflicts; please retry"}, nil
		default:
			return nil, err
		}
	}

	transferID, err := uuid.Parse(result.TransferID)
	if err != nil {
		return nil, fmt.Errorf("httpapi: parse transfer id %q: %w", result.TransferID, err)
	}
	fromID, err := uuid.Parse(result.FromAccountID)
	if err != nil {
		return nil, fmt.Errorf("httpapi: parse from-account id %q: %w", result.FromAccountID, err)
	}
	toID, err := uuid.Parse(result.ToAccountID)
	if err != nil {
		return nil, fmt.Errorf("httpapi: parse to-account id %q: %w", result.ToAccountID, err)
	}

	return PostTransfer200JSONResponse{
		TransferId:       transferID,
		FromAccountId:    fromID,
		ToAccountId:      toID,
		FromBalanceCents: result.FromBalanceCents,
		ToBalanceCents:   result.ToBalanceCents,
		AmountCents:      result.AmountCents,
		Attempts:         int32(result.Attempts),
		CreatedAt:        result.CreatedAt,
	}, nil
}
