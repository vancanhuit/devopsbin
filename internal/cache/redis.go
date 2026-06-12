// Package cache provides access to the Redis instance backing the service.
// For now it exposes only the connection lifecycle and a liveness Ping used by
// the readiness probe; cache operations are added as features land.
//
// The client is topology-agnostic: it wraps a go-redis UniversalClient so the
// rest of the service is unaffected by whether Redis runs as a standalone
// node, a cluster, or behind sentinel. The concrete client is selected from
// the configured RedisConfig.Mode.
package cache

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/vancanhuit/devopsbin/internal/config"
)

// Client wraps a go-redis universal client.
type Client struct {
	rdb redis.UniversalClient
}

// New builds a Redis client for the configured topology. The client connects
// lazily, so a temporarily unreachable Redis does not fail startup; use Ping
// to verify connectivity (e.g. from the readiness probe).
//
// The mode determines the concrete client go-redis constructs:
//   - standalone: a single-node client (exactly one addr).
//   - cluster:    a slot-aware cluster client (addrs are discovery seeds).
//   - sentinel:   a failover client (addrs are sentinel addresses).
func New(cfg config.RedisConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("cache: %w", err)
	}

	opts := &redis.UniversalOptions{
		Addrs:    cfg.Addrs,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	// NewUniversalClient selects the concrete client by these rules:
	// MasterName set -> failover (sentinel); else len(Addrs) > 1 -> cluster;
	// else standalone. Set MasterName only for sentinel, and build a cluster
	// client explicitly for cluster mode so the topology is honored exactly as
	// configured rather than inferred from the addr count.
	switch cfg.Mode {
	case config.RedisSentinel:
		opts.MasterName = cfg.MasterName
	case config.RedisCluster:
		return &Client{rdb: redis.NewClusterClient(opts.Cluster())}, nil
	case config.RedisStandalone:
		// MasterName stays empty; standalone is validated to a single addr.
	default:
		return nil, fmt.Errorf("cache: unknown redis mode %q", cfg.Mode)
	}

	return &Client{rdb: redis.NewUniversalClient(opts)}, nil
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

// IsMiss reports whether err represents a cache miss (key not found) rather
// than a failure, so callers can fall through to the source of truth.
func IsMiss(err error) bool {
	return errors.Is(err, redis.Nil)
}
