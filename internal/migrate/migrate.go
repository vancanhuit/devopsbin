// Package migrate applies the embedded SQL schema migrations to Postgres using
// the goose Provider API.
//
// A Postgres session-level advisory lock (goose session locker) serializes
// concurrent migration runners — e.g. multiple replicas or init containers
// starting at once — so the schema is applied exactly once. Migrations are run
// only via the explicit `devopsbin migrate` command; the server never migrates
// on startup.
//
// goose drives a database/sql *sql.DB (opened with the pgx stdlib driver) and
// manages its own goose_db_version bookkeeping table. The application itself
// continues to use a pgxpool connection pool (see internal/store).
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	// pgx stdlib driver registers the "pgx" database/sql driver used by goose.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/vancanhuit/devopsbin/migrations"
)

// Migrator applies the embedded migrations against a single database. It owns a
// database/sql pool dedicated to migration runs; call Close when finished.
type Migrator struct {
	provider *goose.Provider
	db       *sql.DB
}

// New opens a migration runner for databaseURL. It wires the embedded
// migrations FS into a goose Provider configured with a Postgres session
// locker, so concurrent runners serialize safely. The caller must Close the
// returned Migrator to release the underlying connection.
func New(databaseURL string) (*Migrator, error) {
	if databaseURL == "" {
		return nil, errors.New("migrate: database url is empty")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("migrate: open db: %w", err)
	}

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: new session locker: %w", err)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrations.FS,
		goose.WithSessionLocker(locker),
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: new provider: %w", err)
	}

	return &Migrator{provider: provider, db: db}, nil
}

// Up applies all pending migrations and returns the per-migration results in
// the order they were applied (empty when the database is already up to date).
func (m *Migrator) Up(ctx context.Context) ([]*goose.MigrationResult, error) {
	results, err := m.provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: up: %w", err)
	}
	return results, nil
}

// Status returns the state of every known migration (applied or pending).
func (m *Migrator) Status(ctx context.Context) ([]*goose.MigrationStatus, error) {
	status, err := m.provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: status: %w", err)
	}
	return status, nil
}

// Version reports the current schema version recorded in the database.
func (m *Migrator) Version(ctx context.Context) (int64, error) {
	version, err := m.provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate: version: %w", err)
	}
	return version, nil
}

// Close releases the migration runner's database connection.
func (m *Migrator) Close() error {
	if m == nil || m.db == nil {
		return nil
	}
	if err := m.db.Close(); err != nil {
		return fmt.Errorf("migrate: close db: %w", err)
	}
	return nil
}
