//go:build integration

// Integration tests for the rate limiter against a live Redis. Run via:
//
//	mise run api:test:integration
//
// (which brings up the test-profile infra and sets -tags=integration).
// Requires a live Redis reachable via the loaded config (REDIS_MODE/REDIS_ADDRS),
// which mise.test.toml (MISE_ENV=test) points at the standalone test-profile
// instance (compose service redis-test on localhost:6380).

package ratelimit_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/cache"
	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/ratelimit"
)

func newClient(t *testing.T) *cache.Client {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	c, err := cache.New(cfg.Redis)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// TestLimiter_Allow_RealRedis exercises the limiter end to end against Redis:
// the first calls within the limit are allowed and the call that crosses the
// threshold is rejected with a Retry-After value.
func TestLimiter_Allow_RealRedis(t *testing.T) {
	c := newClient(t)
	limiter := ratelimit.New(c, "test", 3, time.Minute)
	// A unique scope per run keeps the counter isolated from other runs.
	scope := fmt.Sprintf("itest-%d", time.Now().UnixNano())
	ctx := t.Context()

	for i := 1; i <= 3; i++ {
		got := limiter.Allow(ctx, scope)
		if !got.Allowed {
			t.Fatalf("request %d: Allowed = false, want true", i)
		}
		if got.Remaining != 3-i {
			t.Errorf("request %d: Remaining = %d, want %d", i, got.Remaining, 3-i)
		}
	}

	got := limiter.Allow(ctx, scope)
	if got.Allowed {
		t.Fatal("over-limit request: Allowed = true, want false")
	}
	if got.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", got.Remaining)
	}
	if got.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want > 0", got.RetryAfter)
	}
}
