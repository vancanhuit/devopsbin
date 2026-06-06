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
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/store"
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
