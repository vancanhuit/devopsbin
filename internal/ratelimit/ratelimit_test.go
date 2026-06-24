package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeStore is an in-memory Store for the limiter tests. It counts increments
// per key and can be made to fail to exercise the fail-open path.
type fakeStore struct {
	counts map[string]int64
	err    error
}

func newFakeStore() *fakeStore {
	return &fakeStore{counts: make(map[string]int64)}
}

func (f *fakeStore) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.counts[key]++
	return f.counts[key], nil
}

// fixedClock returns a now func pinned to t for deterministic windows.
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestLimiter_Allow_WithinLimit(t *testing.T) {
	store := newFakeStore()
	l := New(store, "test", 3, 10*time.Second)
	// Pin to a time 2s into a 10s window so reset is the remaining 8s.
	l.now = fixedClock(time.Unix(102, 0).UTC())

	for i := 1; i <= 3; i++ {
		got := l.Allow(context.Background(), "1.2.3.4")
		if !got.Allowed {
			t.Fatalf("request %d: Allowed = false, want true", i)
		}
		if got.Limit != 3 {
			t.Errorf("request %d: Limit = %d, want 3", i, got.Limit)
		}
		wantRemaining := 3 - i
		if got.Remaining != wantRemaining {
			t.Errorf("request %d: Remaining = %d, want %d", i, got.Remaining, wantRemaining)
		}
		if got.Reset != 8*time.Second {
			t.Errorf("request %d: Reset = %v, want 8s", i, got.Reset)
		}
		if got.RetryAfter != 0 {
			t.Errorf("request %d: RetryAfter = %v, want 0", i, got.RetryAfter)
		}
	}
}

func TestLimiter_Allow_ExceedsLimit(t *testing.T) {
	store := newFakeStore()
	l := New(store, "test", 2, 10*time.Second)
	l.now = fixedClock(time.Unix(100, 0).UTC())
	ctx := context.Background()

	// Exhaust the allowance.
	for i := 1; i <= 2; i++ {
		if got := l.Allow(ctx, "1.2.3.4"); !got.Allowed {
			t.Fatalf("request %d: Allowed = false, want true", i)
		}
	}

	got := l.Allow(ctx, "1.2.3.4")
	if got.Allowed {
		t.Fatal("over-limit request: Allowed = true, want false")
	}
	if got.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", got.Remaining)
	}
	if got.Reset != 10*time.Second {
		t.Errorf("Reset = %v, want 10s", got.Reset)
	}
	if got.RetryAfter != 10*time.Second {
		t.Errorf("RetryAfter = %v, want 10s", got.RetryAfter)
	}
}

func TestLimiter_Allow_SeparateScopesIndependent(t *testing.T) {
	store := newFakeStore()
	l := New(store, "test", 1, 10*time.Second)
	l.now = fixedClock(time.Unix(100, 0).UTC())
	ctx := context.Background()

	if got := l.Allow(ctx, "1.1.1.1"); !got.Allowed {
		t.Fatal("first scope first request: Allowed = false, want true")
	}
	// A different scope has its own window and is still allowed.
	if got := l.Allow(ctx, "2.2.2.2"); !got.Allowed {
		t.Fatal("second scope first request: Allowed = false, want true")
	}
	// The first scope is now exhausted.
	if got := l.Allow(ctx, "1.1.1.1"); got.Allowed {
		t.Fatal("first scope second request: Allowed = true, want false")
	}
}

func TestLimiter_Allow_NewWindowResets(t *testing.T) {
	store := newFakeStore()
	l := New(store, "test", 1, 10*time.Second)
	ctx := context.Background()

	now := time.Unix(100, 0).UTC()
	l.now = fixedClock(now)
	if got := l.Allow(ctx, "1.2.3.4"); !got.Allowed {
		t.Fatal("window 1 first request: Allowed = false, want true")
	}
	if got := l.Allow(ctx, "1.2.3.4"); got.Allowed {
		t.Fatal("window 1 second request: Allowed = true, want false")
	}

	// Advance into the next aligned window; the key changes so the count resets.
	l.now = fixedClock(now.Add(10 * time.Second))
	if got := l.Allow(ctx, "1.2.3.4"); !got.Allowed {
		t.Fatal("window 2 first request: Allowed = false, want true")
	}
}

func TestLimiter_Allow_FailsOpenOnStoreError(t *testing.T) {
	store := newFakeStore()
	store.err = errors.New("redis down")
	l := New(store, "test", 5, 10*time.Second)
	l.now = fixedClock(time.Unix(100, 0).UTC())

	got := l.Allow(context.Background(), "1.2.3.4")
	if !got.Allowed {
		t.Fatal("Allowed = false on store error, want true (fail open)")
	}
	if got.Limit != 5 || got.Remaining != 5 {
		t.Errorf("Limit/Remaining = %d/%d, want 5/5", got.Limit, got.Remaining)
	}
	if got.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0", got.RetryAfter)
	}
}

func TestSeconds(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want int
	}{
		{"zero rounds up to one", 0, 1},
		{"negative rounds up to one", -5 * time.Second, 1},
		{"sub-second rounds up", 250 * time.Millisecond, 1},
		{"exact second", 2 * time.Second, 2},
		{"fractional rounds up", 2500 * time.Millisecond, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Seconds(tc.in); got != tc.want {
				t.Errorf("Seconds(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
