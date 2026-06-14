// Package store provides access to the PostgreSQL database backing the
// service. It exposes the connection lifecycle, a liveness Ping used by the
// readiness probe, the sqlc-generated query set, and a transaction helper for
// multi-statement units of work.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vancanhuit/devopsbin/internal/store/sqlc"
)

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New parses databaseURL and constructs a connection pool. The pool connects
// lazily, so a temporarily unreachable database does not fail startup; use
// Ping to verify connectivity (e.g. from the readiness probe).
func New(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, errors.New("store: database url is empty")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: parse url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Ping verifies the database is reachable. It acquires a connection from the
// pool and round-trips a no-op query, making it suitable as a readiness check.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Queries returns a sqlc query set bound to the connection pool for
// single-statement, non-transactional access. Use WithTx for multi-statement
// units of work that must be atomic.
func (s *Store) Queries() *sqlc.Queries {
	return sqlc.New(s.pool)
}

// WithTx runs fn inside a single database transaction, passing a sqlc query set
// bound to that transaction. The transaction is rolled back if fn returns an
// error (or panics) and committed otherwise. The deferred rollback after a
// successful commit is a safe no-op.
func (s *Store) WithTx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(sqlc.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit tx: %w", err)
	}
	return nil
}

// Close releases the underlying connection pool. Safe to call on a nil
// receiver to simplify cleanup paths.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}
