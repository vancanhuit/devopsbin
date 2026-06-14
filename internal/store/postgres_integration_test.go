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
