// Package cache provides access to the Redis instance backing the service.
// For now it exposes only the connection lifecycle and a liveness Ping used by
// the readiness probe; cache operations are added as features land.
package cache

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client.
type Client struct {
	rdb *redis.Client
}

// New parses redisURL and constructs a client. The client connects lazily, so
// a temporarily unreachable Redis does not fail startup; use Ping to verify
// connectivity (e.g. from the readiness probe).
func New(redisURL string) (*Client, error) {
	if redisURL == "" {
		return nil, errors.New("cache: redis url is empty")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("cache: parse url: %w", err)
	}
	return &Client{rdb: redis.NewClient(opts)}, nil
}

// Ping verifies Redis is reachable, making it suitable as a readiness check.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close releases the underlying connection pool. Safe to call on a nil
// receiver to simplify cleanup paths.
func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}
