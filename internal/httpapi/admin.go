package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/vancanhuit/devopsbin/internal/store"
)

// GetAdminUsers lists all users. The session and admin-role checks are enforced
// by the auth middleware before this runs.
func (s *Server) GetAdminUsers(ctx context.Context, _ GetAdminUsersRequestObject) (GetAdminUsersResponseObject, error) {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminUser, 0, len(users))
	for _, u := range users {
		id, err := uuid.Parse(u.ID)
		if err != nil {
			return nil, fmt.Errorf("httpapi: parse user id %q: %w", u.ID, err)
		}
		out = append(out, AdminUser{
			Id:        id,
			Username:  u.Username,
			Role:      AdminUserRole(u.Role),
			CreatedAt: u.CreatedAt,
		})
	}
	return GetAdminUsers200JSONResponse{Users: out}, nil
}

// GetAdminAccounts lists every account across all users with the owner's
// username.
func (s *Server) GetAdminAccounts(ctx context.Context, _ GetAdminAccountsRequestObject) (GetAdminAccountsResponseObject, error) {
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
	return GetAdminAccounts200JSONResponse{Accounts: out}, nil
}

// GetAdminTransfers lists the transfers ledger, most recent first.
func (s *Server) GetAdminTransfers(ctx context.Context, _ GetAdminTransfersRequestObject) (GetAdminTransfersResponseObject, error) {
	transfers, err := s.users.ListTransfers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminTransfer, 0, len(transfers))
	for _, t := range transfers {
		id, err := uuid.Parse(t.ID)
		if err != nil {
			return nil, fmt.Errorf("httpapi: parse transfer id %q: %w", t.ID, err)
		}
		fromID, err := uuid.Parse(t.FromAccountID)
		if err != nil {
			return nil, fmt.Errorf("httpapi: parse from-account id %q: %w", t.FromAccountID, err)
		}
		toID, err := uuid.Parse(t.ToAccountID)
		if err != nil {
			return nil, fmt.Errorf("httpapi: parse to-account id %q: %w", t.ToAccountID, err)
		}
		out = append(out, AdminTransfer{
			Id:              id,
			FromAccountId:   fromID,
			FromAccountName: t.FromAccountName,
			ToAccountId:     toID,
			ToAccountName:   t.ToAccountName,
			AmountCents:     t.AmountCents,
			CreatedAt:       t.CreatedAt,
		})
	}
	return GetAdminTransfers200JSONResponse{Transfers: out}, nil
}

// PostAdminUserUnlock clears the brute-force login lockout for the target user.
func (s *Server) PostAdminUserUnlock(ctx context.Context, request PostAdminUserUnlockRequestObject) (PostAdminUserUnlockResponseObject, error) {
	user, err := s.users.UserByID(ctx, request.Id.String())
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return PostAdminUserUnlock404JSONResponse{Error: "user not found"}, nil
		}
		return nil, err
	}
	if s.lockout != nil {
		if err := s.lockout.Unlock(ctx, user.Username); err != nil {
			return nil, err
		}
	}
	return PostAdminUserUnlock204Response{}, nil
}

// PostAdminUserPasswordReset mints a single-use reset token for the target
// user. As with the self-service reset-request endpoint, the token is returned
// in the response body for this demo (production would email it).
func (s *Server) PostAdminUserPasswordReset(ctx context.Context, request PostAdminUserPasswordResetRequestObject) (PostAdminUserPasswordResetResponseObject, error) {
	user, err := s.users.UserByID(ctx, request.Id.String())
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return PostAdminUserPasswordReset404JSONResponse{Error: "user not found"}, nil
		}
		return nil, err
	}

	const ack = "a reset token has been issued for the user"
	if s.recovery == nil {
		return PostAdminUserPasswordReset200JSONResponse{Message: ack}, nil
	}

	token, err := s.recovery.CreateResetToken(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return PostAdminUserPasswordReset200JSONResponse{Message: ack, Token: &token}, nil
}
