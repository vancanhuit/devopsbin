//go:build integration

// Integration tests for the migration runner. Run via:
//
//	mise run api:test:integration
//
// (which brings up the test-profile Postgres and sets -tags=integration).
// Requires a live Postgres reachable via the loaded config (POSTGRES_URL),
// which the api:test:integration task points at the test-profile instance.

package migrate_test

import (
	"context"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/migrate"
	"github.com/vancanhuit/devopsbin/migrations"
)

// expectedMigrations is the number of *.sql files embedded under migrations/.
// It is derived from the embedded FS so adding a migration updates it
// automatically.
func expectedMigrations(t *testing.T) int {
	t.Helper()
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	return count
}

func newMigrator(t *testing.T) *migrate.Migrator {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	m, err := migrate.New(cfg.Postgres.URL)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// TestMigrator_Up_IsIdempotent applies all migrations and verifies that a
// second Up is a no-op, the schema version matches the migration count, and
// every migration reports as applied. It does not assert that the first Up did
// the work, because another integration package may have already applied the
// schema against the shared test database — the migration runner's session
// lock makes that safe.
func TestMigrator_Up_IsIdempotent(t *testing.T) {
	m := newMigrator(t)
	want := expectedMigrations(t)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	if _, err := m.Up(ctx); err != nil {
		t.Fatalf("first Up: %v", err)
	}

	results, err := m.Up(ctx)
	if err != nil {
		t.Fatalf("second Up: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("second Up applied %d migrations, want 0 (idempotent)", len(results))
	}

	version, err := m.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if version != int64(want) {
		t.Fatalf("schema version = %d, want %d", version, want)
	}

	status, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(status) != want {
		t.Fatalf("status reported %d migrations, want %d", len(status), want)
	}
	for _, s := range status {
		if s.AppliedAt.IsZero() {
			t.Fatalf("migration %d not applied", s.Source.Version)
		}
	}
}
