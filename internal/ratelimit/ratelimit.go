// Package ratelimit implements a Redis-backed fixed-window rate limiter used by
// the rate-limit demo endpoint.
//
// The limiter counts requests per client within aligned wall-clock windows: a
// key encodes the caller scope and the window's start second, and a single
// atomic INCR (with the window TTL set on the first increment) advances the
// counter. Because the counter lives in Redis, the limit is enforced
// consistently across every stateless replica rather than per process.
//
// It is a best-effort defense: when the store is unavailable the limiter fails
// open (allows the request) so a cache outage degrades to no limiting rather
// than rejecting every caller, mirroring the project's Redis guidance.
package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// keyPrefix namespaces every limiter key so the format can evolve without
// colliding with other Redis users.
const keyPrefix = "ratelimit:v1:"

// Store is the subset of cache operations the limiter needs. The cache client
// satisfies it; Incr must increment the counter and set its window TTL on the
// first increment so the window is anchored and self-expiring.
type Store interface {
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// Result reports the outcome of a single limiter check and the values needed to
// populate the standard RateLimit response headers.
type Result struct {
	// Allowed is true when the request is within the limit and should proceed.
	Allowed bool
	// Limit is the maximum number of requests permitted within the window.
	Limit int
	// Remaining is the number of requests left in the current window (never
	// negative).
	Remaining int
	// Reset is the time until the current window ends.
	Reset time.Duration
	// RetryAfter is how long the caller should wait before retrying. It is zero
	// unless the request was rejected.
	RetryAfter time.Duration
}

// Limiter enforces a fixed-window request limit per caller scope.
type Limiter struct {
	store  Store
	name   string
	limit  int
	window time.Duration
	// now returns the current time; it is a field so tests can supply a fixed
	// clock.
	now func() time.Time
}

// New builds a Limiter. name namespaces the keys (e.g. the route being limited),
// limit is the maximum requests per window, and window is the fixed window
// length. limit must be >= 1 and window must be positive.
func New(store Store, name string, limit int, window time.Duration) *Limiter {
	return &Limiter{
		store:  store,
		name:   name,
		limit:  limit,
		window: window,
		now:    time.Now,
	}
}

// Allow records a request from scope (e.g. a client IP) against the current
// window and reports whether it is permitted along with the values for the
// RateLimit headers. A store error fails open: the request is allowed and the
// headers report a full allowance for the window.
func (l *Limiter) Allow(ctx context.Context, scope string) Result {
	now := l.now()
	windowStart := now.Truncate(l.window)
	reset := windowStart.Add(l.window).Sub(now)
	key := fmt.Sprintf("%s%s:%s:%d", keyPrefix, l.name, scope, windowStart.Unix())

	n, err := l.store.Incr(ctx, key, l.window)
	if err != nil {
		// Fail open: never reject a caller because the cache is unavailable.
		return Result{Allowed: true, Limit: l.limit, Remaining: l.limit, Reset: reset}
	}

	remaining := l.limit - int(n)
	if remaining < 0 {
		remaining = 0
	}
	if n > int64(l.limit) {
		return Result{
			Allowed:    false,
			Limit:      l.limit,
			Remaining:  0,
			Reset:      reset,
			RetryAfter: reset,
		}
	}
	return Result{Allowed: true, Limit: l.limit, Remaining: remaining, Reset: reset}
}

// Seconds converts a duration to whole seconds for a RateLimit-Reset or
// Retry-After header, rounding up and reporting at least one second so a client
// never busy-retries against a sub-second remainder.
func Seconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return secs
}
