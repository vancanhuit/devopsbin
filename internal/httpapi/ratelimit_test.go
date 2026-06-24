package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
	"github.com/vancanhuit/devopsbin/internal/ratelimit"
)

// countingStore is an in-memory ratelimit.Store for the handler tests: it
// counts increments per key so the limiter behaves deterministically without
// Redis.
type countingStore struct {
	counts map[string]int64
}

func newCountingStore() *countingStore {
	return &countingStore{counts: make(map[string]int64)}
}

func (s *countingStore) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	s.counts[key]++
	return s.counts[key], nil
}

func TestGetRatelimit_WithinLimit_OK(t *testing.T) {
	limiter := ratelimit.New(newCountingStore(), "ratelimit", 2, 10*time.Second)
	h := httpapi.NewServer(httpapi.WithRateLimiter(limiter)).Handler()

	rec := doGet(t, h, "/api/v1/ratelimit")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decode[httpapi.RateLimitResponse](t, rec)
	if body.Limit != 2 {
		t.Errorf("limit = %d, want 2", body.Limit)
	}
	if body.Remaining != 1 {
		t.Errorf("remaining = %d, want 1", body.Remaining)
	}
	if got := rec.Header().Get("RateLimit-Limit"); got != "2" {
		t.Errorf("RateLimit-Limit header = %q, want 2", got)
	}
	if got := rec.Header().Get("RateLimit-Remaining"); got != "1" {
		t.Errorf("RateLimit-Remaining header = %q, want 1", got)
	}
	if got := rec.Header().Get("RateLimit-Reset"); got == "" {
		t.Error("RateLimit-Reset header missing")
	}
	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Errorf("Retry-After header = %q, want empty on 200", got)
	}
}

func TestGetRatelimit_ExceedsLimit_429(t *testing.T) {
	limiter := ratelimit.New(newCountingStore(), "ratelimit", 1, 10*time.Second)
	h := httpapi.NewServer(httpapi.WithRateLimiter(limiter)).Handler()

	// First request consumes the single allowance.
	if rec := doGet(t, h, "/api/v1/ratelimit"); rec.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", rec.Code)
	}

	rec := doGet(t, h, "/api/v1/ratelimit")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want 429", rec.Code)
	}
	body := decode[httpapi.ErrorResponse](t, rec)
	if body.Error == "" {
		t.Error("error body empty, want a message")
	}
	if got := rec.Header().Get("RateLimit-Remaining"); got != "0" {
		t.Errorf("RateLimit-Remaining header = %q, want 0", got)
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Error("Retry-After header missing on 429")
	}
}

func TestGetRatelimit_NoLimiter_Allows(t *testing.T) {
	// Without a configured limiter the endpoint never throttles.
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/ratelimit")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// TestGetRatelimit_PerClientIP verifies the limiter keys on the client IP so
// distinct callers do not share an allowance.
func TestGetRatelimit_PerClientIP(t *testing.T) {
	limiter := ratelimit.New(newCountingStore(), "ratelimit", 1, 10*time.Second)
	h := httpapi.NewServer(httpapi.WithRateLimiter(limiter)).Handler()

	get := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ratelimit", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := get("198.51.100.1:1111"); code != http.StatusOK {
		t.Fatalf("client A first request = %d, want 200", code)
	}
	// A different client IP has its own window and is still allowed.
	if code := get("198.51.100.2:2222"); code != http.StatusOK {
		t.Fatalf("client B first request = %d, want 200", code)
	}
	// Client A is now over its limit.
	if code := get("198.51.100.1:1111"); code != http.StatusTooManyRequests {
		t.Fatalf("client A second request = %d, want 429", code)
	}
}
