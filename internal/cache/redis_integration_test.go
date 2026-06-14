//go:build integration

// Integration tests for the Redis cache wrapper. Run via:
//
//	mise run api:test:integration
//
// (which brings up the test-profile infra and sets -tags=integration).
// Requires a live Redis reachable via the loaded config (REDIS_MODE/REDIS_ADDRS),
// which mise.test.toml (MISE_ENV=test) points at the standalone test-profile
// instance (compose service redis-test on localhost:6380).

package cache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/cache"
	"github.com/vancanhuit/devopsbin/internal/config"
)

// testRedisConfig returns the Redis settings for the test instance, loaded
// through the same config package used in production. The mise
// api:test:integration task sets REDIS_MODE/REDIS_ADDRS via mise.test.toml
// (MISE_ENV=test) to the standalone test-profile instance (compose service
// redis-test on localhost:6380).
func testRedisConfig(t *testing.T) config.RedisConfig {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg.Redis
}

func newClient(t *testing.T) *cache.Client {
	t.Helper()
	c, err := cache.New(testRedisConfig(t))
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
	// A valid standalone addr pointing at a port nothing listens on. The
	// client connects lazily, so New succeeds and Ping surfaces the failure.
	c, err := cache.New(config.RedisConfig{
		Mode:  config.RedisStandalone,
		Addrs: []string{"localhost:1"},
	})
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

// TestClient_SetGetDel exercises the session-store primitives: a value set with
// a TTL round-trips, and Del removes it so a subsequent Get reports a miss.
func TestClient_SetGetDel(t *testing.T) {
	c := newClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("test:cache:%d", time.Now().UnixNano())
	const value = "hello"

	if err := c.Set(ctx, key, value, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != value {
		t.Fatalf("Get = %q, want %q", got, value)
	}

	if err := c.Del(ctx, key); err != nil {
		t.Fatalf("Del: %v", err)
	}

	if _, err := c.Get(ctx, key); !cache.IsMiss(err) {
		t.Fatalf("Get after Del err = %v, want a miss", err)
	}
}

// TestClient_Get_Miss reports a miss for an absent key.
func TestClient_Get_Miss(t *testing.T) {
	c := newClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("test:cache:absent:%d", time.Now().UnixNano())
	if _, err := c.Get(ctx, key); !cache.IsMiss(err) {
		t.Fatalf("Get err = %v, want a miss", err)
	}
}

// TestClient_GetDel atomically returns and deletes a key; a second GetDel of
// the same key reports a miss.
func TestClient_GetDel(t *testing.T) {
	c := newClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("test:cache:getdel:%d", time.Now().UnixNano())
	if err := c.Set(ctx, key, "once", time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := c.GetDel(ctx, key)
	if err != nil {
		t.Fatalf("GetDel: %v", err)
	}
	if got != "once" {
		t.Fatalf("GetDel = %q, want %q", got, "once")
	}
	if _, err := c.GetDel(ctx, key); !cache.IsMiss(err) {
		t.Fatalf("second GetDel err = %v, want a miss", err)
	}
}

// TestClient_Incr increments a fixed-window counter and reports the remaining
// TTL set on the first increment.
func TestClient_Incr(t *testing.T) {
	c := newClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("test:cache:incr:%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = c.Del(context.Background(), key) })

	n, err := c.Incr(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if n != 1 {
		t.Fatalf("first Incr = %d, want 1", n)
	}
	n, err = c.Incr(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if n != 2 {
		t.Fatalf("second Incr = %d, want 2", n)
	}

	ttl, err := c.TTL(ctx, key)
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %v, want (0, 1m]", ttl)
	}
}

// TestClient_TTL_NoExpiry reports a zero duration for a key without an expiry,
// and TestClient_TTL_Missing does likewise for an absent key.
func TestClient_TTL_NoExpiryAndMissing(t *testing.T) {
	c := newClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	missing := fmt.Sprintf("test:cache:ttl:absent:%d", time.Now().UnixNano())
	if ttl, err := c.TTL(ctx, missing); err != nil || ttl != 0 {
		t.Fatalf("TTL(absent) = %v, %v; want 0, nil", ttl, err)
	}

	noExpiry := fmt.Sprintf("test:cache:ttl:persist:%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = c.Del(context.Background(), noExpiry) })
	if err := c.Set(ctx, noExpiry, "v", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if ttl, err := c.TTL(ctx, noExpiry); err != nil || ttl != 0 {
		t.Fatalf("TTL(no-expiry) = %v, %v; want 0, nil", ttl, err)
	}
}

// TestClient_SAddSMembers adds members to a set and reads them back; an absent
// set reads as empty.
func TestClient_SAddSMembers(t *testing.T) {
	c := newClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	key := fmt.Sprintf("test:cache:set:%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = c.Del(context.Background(), key) })

	if err := c.SAdd(ctx, key, "a", time.Minute); err != nil {
		t.Fatalf("SAdd a: %v", err)
	}
	if err := c.SAdd(ctx, key, "b", time.Minute); err != nil {
		t.Fatalf("SAdd b: %v", err)
	}

	members, err := c.SMembers(ctx, key)
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	got := map[string]bool{}
	for _, m := range members {
		got[m] = true
	}
	if len(got) != 2 || !got["a"] || !got["b"] {
		t.Fatalf("SMembers = %v, want {a, b}", members)
	}

	absent := fmt.Sprintf("test:cache:set:absent:%d", time.Now().UnixNano())
	empty, err := c.SMembers(ctx, absent)
	if err != nil {
		t.Fatalf("SMembers(absent): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("SMembers(absent) = %v, want empty", empty)
	}
}
