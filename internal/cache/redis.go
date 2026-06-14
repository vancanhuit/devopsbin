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
	"time"

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

// Set stores value at key with an expiry. A non-positive ttl stores the key
// without an expiry; callers should always pass a positive ttl so keys cannot
// live forever.
func (c *Client) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache: set %q: %w", key, err)
	}
	return nil
}

// Get returns the value stored at key. When the key is absent it returns the
// underlying redis.Nil error; callers distinguish a miss from a failure with
// IsMiss.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	v, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if IsMiss(err) {
			return "", err
		}
		return "", fmt.Errorf("cache: get %q: %w", key, err)
	}
	return v, nil
}

// Del removes key. Deleting a missing key is not an error.
func (c *Client) Del(ctx context.Context, key string) error {
	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("cache: del %q: %w", key, err)
	}
	return nil
}

// GetDel atomically returns the value at key and deletes it, giving a key a
// single-use semantics (e.g. a one-shot password-reset token). When the key is
// absent it returns the underlying redis.Nil error; callers distinguish a miss
// from a failure with IsMiss.
func (c *Client) GetDel(ctx context.Context, key string) (string, error) {
	v, err := c.rdb.GetDel(ctx, key).Result()
	if err != nil {
		if IsMiss(err) {
			return "", err
		}
		return "", fmt.Errorf("cache: getdel %q: %w", key, err)
	}
	return v, nil
}

// incrWindowScript atomically increments key and, only on the first increment
// (when the new value is 1), sets its expiry to ARGV[1] milliseconds. This
// implements a fixed window that starts at the first event and cannot be
// extended by later increments, avoiding a separate non-atomic EXPIRE call.
var incrWindowScript = redis.NewScript(`
local v = redis.call("INCR", KEYS[1])
if v == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return v
`)

// Incr atomically increments the integer counter at key and returns the new
// value. On the first increment it sets the key to expire after ttl, so the
// counter behaves as a fixed window anchored at the first event. A non-positive
// ttl increments without setting an expiry.
func (c *Client) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		n, err := c.rdb.Incr(ctx, key).Result()
		if err != nil {
			return 0, fmt.Errorf("cache: incr %q: %w", key, err)
		}
		return n, nil
	}
	n, err := incrWindowScript.Run(ctx, c.rdb, []string{key}, ttl.Milliseconds()).Int64()
	if err != nil {
		return 0, fmt.Errorf("cache: incr %q: %w", key, err)
	}
	return n, nil
}

// TTL returns the remaining time to live of key. A missing key or a key with no
// associated expiry both report a zero duration, so callers can treat "no
// deadline" uniformly.
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := c.rdb.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("cache: ttl %q: %w", key, err)
	}
	if d < 0 {
		return 0, nil
	}
	return d, nil
}

// SAdd adds member to the set at key and refreshes the set's expiry to ttl in a
// single round trip. Tracking a bounded, self-expiring set (e.g. a user's
// active session ids) lets callers enumerate and revoke them without scanning
// the keyspace. A non-positive ttl adds the member without setting an expiry.
func (c *Client) SAdd(ctx context.Context, key, member string, ttl time.Duration) error {
	_, err := c.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SAdd(ctx, key, member)
		if ttl > 0 {
			pipe.PExpire(ctx, key, ttl)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cache: sadd %q: %w", key, err)
	}
	return nil
}

// SMembers returns the members of the set at key. A missing set returns an
// empty slice and no error.
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	members, err := c.rdb.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("cache: smembers %q: %w", key, err)
	}
	return members, nil
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
