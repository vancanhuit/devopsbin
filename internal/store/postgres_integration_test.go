//go:build integration

// Integration tests for the Postgres store wrapper. Run via:
//
//	mise run api:test:integration
//
// (which brings up the test-profile infra and sets -tags=integration).
// Requires a live Postgres reachable via the loaded config (POSTGRES_URL),
// which mise.test.toml (MISE_ENV=test) points at the test-profile instance.

package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/migrate"
	"github.com/vancanhuit/devopsbin/internal/store"
	"github.com/vancanhuit/devopsbin/internal/store/sqlc"
)

// testPostgresURL returns the DSN of the test Postgres, loaded through the same
// config package used in production. The mise api:test:integration task sets
// POSTGRES_URL via mise.test.toml (MISE_ENV=test) to the test-profile instance
// (compose service postgres-test on localhost:5433).
func testPostgresURL(t *testing.T) string {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg.Postgres.URL
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	url := testPostgresURL(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	s, err := store.New(ctx, url)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestStore_Ping(t *testing.T) {
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestStore_Ping_Unreachable(t *testing.T) {
	// A syntactically valid DSN pointing at a port nothing listens on. The
	// pool connects lazily, so New succeeds and Ping surfaces the failure.
	const dsn = "postgres://devopsbin:devopsbin@localhost:1/devopsbin?sslmode=disable"
	s, err := store.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(s.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err == nil {
		t.Fatal("expected ping to fail against an unreachable database")
	}
}

// applyMigrations brings the shared test database up to the latest schema so
// the query and transaction tests have tables to work against. It is idempotent
// and safe to call from multiple tests (the runner takes a session lock).
func applyMigrations(t *testing.T) {
	t.Helper()
	m, err := migrate.New(testPostgresURL(t))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	defer func() { _ = m.Close() }()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("migrate Up: %v", err)
	}
}

// uniqueUsername returns a username unlikely to collide with the seeded demo
// users or with other tests running against the shared test database.
func uniqueUsername(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-%d", time.Now().UnixNano())
}

// TestStore_WithTx_Commit creates a user and an owned account in a single
// transaction and verifies both are persisted and readable afterwards.
func TestStore_WithTx_Commit(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	var userID pgtype.UUID

	err := s.WithTx(ctx, func(q *sqlc.Queries) error {
		user, err := q.CreateUser(ctx, sqlc.CreateUserParams{
			Username:     username,
			PasswordHash: "x",
			Role:         "user",
		})
		if err != nil {
			return err
		}
		userID = user.ID

		_, err = q.CreateAccount(ctx, sqlc.CreateAccountParams{
			UserID:       user.ID,
			Name:         "Checking",
			BalanceCents: 500,
		})
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	user, err := s.Queries().GetUserByUsername(ctx, username)
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if user.ID != userID {
		t.Fatalf("user id = %v, want %v", user.ID, userID)
	}
	if user.Role != "user" {
		t.Fatalf("role = %q, want %q", user.Role, "user")
	}

	accounts, err := s.Queries().ListAccountsByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListAccountsByUser: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("got %d accounts, want 1", len(accounts))
	}
	if accounts[0].BalanceCents != 500 {
		t.Fatalf("balance = %d, want 500", accounts[0].BalanceCents)
	}
}

// TestStore_WithTx_Rollback verifies that returning an error from the
// transaction function rolls back every statement, so a partially-applied unit
// of work leaves no rows behind.
func TestStore_WithTx_Rollback(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	sentinel := errors.New("boom")

	err := s.WithTx(ctx, func(q *sqlc.Queries) error {
		if _, err := q.CreateUser(ctx, sqlc.CreateUserParams{
			Username:     username,
			PasswordHash: "x",
			Role:         "user",
		}); err != nil {
			return err
		}
		// Abort after a successful insert; the commit must not happen.
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithTx error = %v, want %v", err, sentinel)
	}

	if _, err := s.Queries().GetUserByUsername(ctx, username); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected user to be rolled back (pgx.ErrNoRows), got %v", err)
	}
}

// TestStore_RegisterUser_CreatesUserAndAccount registers a new user and
// verifies the user, a starter account, and the canonical id round-trip.
func TestStore_RegisterUser_CreatesUserAndAccount(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	user, err := s.RegisterUser(ctx, store.NewUser{
		Username:     username,
		PasswordHash: "hash",
		Role:         "user",
	})
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}
	if user.Username != username || user.Role != "user" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if user.ID == "" {
		t.Fatal("expected a non-empty canonical user id")
	}

	got, err := s.UserByUsername(ctx, username)
	if err != nil {
		t.Fatalf("UserByUsername: %v", err)
	}
	if got.ID != user.ID || got.PasswordHash != "hash" {
		t.Fatalf("UserByUsername mismatch: %+v", got)
	}

	id, err := uuid.Parse(user.ID)
	if err != nil {
		t.Fatalf("user id %q is not a valid uuid: %v", user.ID, err)
	}
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	accounts, err := s.Queries().ListAccountsByUser(ctx, pgID)
	if err != nil {
		t.Fatalf("ListAccountsByUser: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("got %d starter accounts, want 1", len(accounts))
	}
}

// TestStore_RegisterUser_DuplicateUsername maps a unique-constraint violation
// to ErrUsernameTaken.
func TestStore_RegisterUser_DuplicateUsername(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	params := store.NewUser{Username: username, PasswordHash: "hash", Role: "user"}
	if _, err := s.RegisterUser(ctx, params); err != nil {
		t.Fatalf("first RegisterUser: %v", err)
	}
	if _, err := s.RegisterUser(ctx, params); !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
}

// TestStore_UserByUsername_NotFound maps a missing user to ErrUserNotFound.
func TestStore_UserByUsername_NotFound(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	if _, err := s.UserByUsername(ctx, uniqueUsername(t)); !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

// TestStore_UserByID_RoundTrip looks a registered user up by id.
func TestStore_UserByID_RoundTrip(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	user, err := s.RegisterUser(ctx, store.NewUser{Username: username, PasswordHash: "hash", Role: "user"})
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	got, err := s.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if got.ID != user.ID || got.Username != username || got.PasswordHash != "hash" {
		t.Fatalf("UserByID mismatch: %+v", got)
	}
}

// TestStore_UserByID_NotFound maps a malformed or absent id to ErrUserNotFound.
func TestStore_UserByID_NotFound(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	if _, err := s.UserByID(ctx, "not-a-uuid"); !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("malformed id err = %v, want ErrUserNotFound", err)
	}
	if _, err := s.UserByID(ctx, "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed"); !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("absent id err = %v, want ErrUserNotFound", err)
	}
}

// TestStore_UpdatePassword changes the stored hash and maps unknown users to
// ErrUserNotFound.
func TestStore_UpdatePassword(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	user, err := s.RegisterUser(ctx, store.NewUser{Username: username, PasswordHash: "old-hash", Role: "user"})
	if err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	if err := s.UpdatePassword(ctx, user.ID, "new-hash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	got, err := s.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if got.PasswordHash != "new-hash" {
		t.Fatalf("password hash = %q, want %q", got.PasswordHash, "new-hash")
	}

	if err := s.UpdatePassword(ctx, "not-a-uuid", "x"); !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("malformed id err = %v, want ErrUserNotFound", err)
	}
	if err := s.UpdatePassword(ctx, "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed", "x"); !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("absent id err = %v, want ErrUserNotFound", err)
	}
}

// TestStore_SeedData verifies the seed migration created the documented demo
// users with their roles.
func TestStore_SeedData(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	for username, wantRole := range map[string]string{"alice": "user", "admin": "admin"} {
		user, err := s.Queries().GetUserByUsername(ctx, username)
		if err != nil {
			t.Fatalf("GetUserByUsername(%q): %v", username, err)
		}
		if user.Role != wantRole {
			t.Fatalf("user %q role = %q, want %q", username, user.Role, wantRole)
		}

		accounts, err := s.Queries().ListAccountsByUser(ctx, user.ID)
		if err != nil {
			t.Fatalf("ListAccountsByUser(%q): %v", username, err)
		}
		if len(accounts) == 0 {
			t.Fatalf("user %q has no seeded account", username)
		}
	}
}

// TestStore_ListUsers returns every user with id, username, and role. It must
// include the seeded demo users and any freshly registered one.
func TestStore_ListUsers(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	if _, err := s.RegisterUser(ctx, store.NewUser{Username: username, PasswordHash: "hash", Role: "user"}); err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	roles := make(map[string]string, len(users))
	for _, u := range users {
		roles[u.Username] = u.Role
		if u.ID == "" {
			t.Fatalf("user %q has an empty id", u.Username)
		}
		if _, err := uuid.Parse(u.ID); err != nil {
			t.Fatalf("user %q id %q is not a valid uuid: %v", u.Username, u.ID, err)
		}
		if u.CreatedAt.IsZero() {
			t.Fatalf("user %q has a zero created_at", u.Username)
		}
	}
	if roles["alice"] != "user" || roles["admin"] != "admin" || roles[username] != "user" {
		t.Fatalf("unexpected roles: %+v", roles)
	}
}

// TestStore_ListAllAccounts returns every account across users joined to its
// owner's username.
func TestStore_ListAllAccounts(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	username := uniqueUsername(t)
	if _, err := s.RegisterUser(ctx, store.NewUser{Username: username, PasswordHash: "hash", Role: "user"}); err != nil {
		t.Fatalf("RegisterUser: %v", err)
	}

	accounts, err := s.ListAllAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAllAccounts: %v", err)
	}
	owners := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		owners[a.OwnerUsername] = true
		if a.ID == "" {
			t.Fatalf("account for %q has an empty id", a.OwnerUsername)
		}
		if a.Name == "" {
			t.Fatalf("account for %q has an empty name", a.OwnerUsername)
		}
	}
	if !owners["alice"] || !owners["admin"] || !owners[username] {
		t.Fatalf("expected accounts owned by alice, admin, and %q; got owners %+v", username, owners)
	}
}

// TestStore_ListTransfers returns the transfers ledger joined to both account
// names. It inserts a transfer between the two seeded accounts and asserts it
// is listed with the expected fields.
func TestStore_ListTransfers(t *testing.T) {
	applyMigrations(t)
	s := newStore(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, testPostgresURL(t))
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	var transferID string
	err = conn.QueryRow(ctx, `
		INSERT INTO transfers (from_account_id, to_account_id, amount_cents)
		SELECT
			(SELECT a.id FROM accounts a JOIN users u ON u.id = a.user_id WHERE u.username = 'alice'),
			(SELECT a.id FROM accounts a JOIN users u ON u.id = a.user_id WHERE u.username = 'admin'),
			777
		RETURNING id::text
	`).Scan(&transferID)
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	transfers, err := s.ListTransfers(ctx)
	if err != nil {
		t.Fatalf("ListTransfers: %v", err)
	}

	var found *store.AdminTransfer
	for i := range transfers {
		if transfers[i].ID == transferID {
			found = &transfers[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("inserted transfer %q not found in %d listed transfers", transferID, len(transfers))
	}
	if found.AmountCents != 777 {
		t.Fatalf("amount_cents = %d, want 777", found.AmountCents)
	}
	if found.FromAccountName != "Checking" || found.ToAccountName != "Checking" {
		t.Fatalf("account names = %q -> %q, want Checking -> Checking", found.FromAccountName, found.ToAccountName)
	}
	if found.FromAccountID == "" || found.ToAccountID == "" {
		t.Fatalf("transfer has empty account ids: from=%q to=%q", found.FromAccountID, found.ToAccountID)
	}
	if found.CreatedAt.IsZero() {
		t.Fatal("transfer has a zero created_at")
	}
}
