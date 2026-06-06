// Package store provides access to the PostgreSQL database backing the
// service. For now it exposes only the connection lifecycle and a liveness
// Ping used by the readiness probe; query methods are added as features land.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
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

// Close releases the underlying connection pool. Safe to call on a nil
// receiver to simplify cleanup paths.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}
