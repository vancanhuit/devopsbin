//go:build integration

// Integration tests for the Redis cache wrapper. Run via:
//
//	mise run api:test:integration
//
// (which brings up the test-profile infra and sets -tags=integration).
// Requires a live Redis reachable via the loaded config (REDIS_URL), which
// mise.test.toml (MISE_ENV=test) points at the test-profile instance.

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/cache"
	"github.com/vancanhuit/devopsbin/internal/config"
)

// testRedisURL returns the URL of the test Redis, loaded through the same config
// package used in production. The mise api:test:integration task sets REDIS_URL
// via mise.test.toml (MISE_ENV=test) to the test-profile instance (compose
// service redis-test on localhost:6380).
func testRedisURL(t *testing.T) string {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg.Redis.URL
}

func newClient(t *testing.T) *cache.Client {
	t.Helper()
	c, err := cache.New(testRedisURL(t))
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestClient_Ping(t *testing.T) {
	c := newClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClient_Ping_Unreachable(t *testing.T) {
	// A syntactically valid URL pointing at a port nothing listens on. The
	// client connects lazily, so New succeeds and Ping surfaces the failure.
	c, err := cache.New("redis://localhost:1/0")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	if err := c.Ping(ctx); err == nil {
		t.Fatal("expected ping to fail against an unreachable redis")
	}
}
