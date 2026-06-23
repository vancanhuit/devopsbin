package store

import (
	"context"
	"fmt"
	"time"
)

// AdminUser is the admin view of a user: identity and role without the password
// hash, plus the creation time for the admin listing.
type AdminUser struct {
	ID        string
	Username  string
	Role      string
	CreatedAt time.Time
}

// AdminAccount is the admin view of an account joined to its owner's username.
type AdminAccount struct {
	ID            string
	OwnerUsername string
	Name          string
	BalanceCents  int64
	CreatedAt     time.Time
}

// AdminTransfer is the admin view of a ledger transfer joined to the source and
// destination account names.
type AdminTransfer struct {
	ID              string
	FromAccountID   string
	FromAccountName string
	ToAccountID     string
	ToAccountName   string
	AmountCents     int64
	CreatedAt       time.Time
}

// ListUsers returns every user ordered by creation time, for the admin surface.
func (s *Store) ListUsers(ctx context.Context) ([]AdminUser, error) {
	rows, err := s.Queries().ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	users := make([]AdminUser, 0, len(rows))
	for _, row := range rows {
		users = append(users, AdminUser{
			ID:        uuidString(row.ID),
			Username:  row.Username,
			Role:      row.Role,
			CreatedAt: row.CreatedAt.Time,
		})
	}
	return users, nil
}

// ListAllAccounts returns every account across all users, joined to the owner's
// username, for the admin surface.
func (s *Store) ListAllAccounts(ctx context.Context) ([]AdminAccount, error) {
	rows, err := s.Queries().ListAllAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list accounts: %w", err)
	}
	accounts := make([]AdminAccount, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, AdminAccount{
			ID:            uuidString(row.ID),
			OwnerUsername: row.OwnerUsername,
			Name:          row.Name,
			BalanceCents:  row.BalanceCents,
			CreatedAt:     row.CreatedAt.Time,
		})
	}
	return accounts, nil
}

// ListTransfers returns the transfers ledger, most recent first, joined to the
// source and destination account names, for the admin surface.
func (s *Store) ListTransfers(ctx context.Context) ([]AdminTransfer, error) {
	rows, err := s.Queries().ListTransfers(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: list transfers: %w", err)
	}
	transfers := make([]AdminTransfer, 0, len(rows))
	for _, row := range rows {
		transfers = append(transfers, AdminTransfer{
			ID:              uuidString(row.ID),
			FromAccountID:   uuidString(row.FromAccountID),
			FromAccountName: row.FromAccountName,
			ToAccountID:     uuidString(row.ToAccountID),
			ToAccountName:   row.ToAccountName,
			AmountCents:     row.AmountCents,
			CreatedAt:       row.CreatedAt.Time,
		})
	}
	return transfers, nil
}
