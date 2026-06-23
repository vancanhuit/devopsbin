package auth

import (
	"context"
	"testing"
	"time"
)

const (
	testLockMax    = 3
	testLockWindow = 15 * time.Minute
	testLockTTL    = 10 * time.Minute
)

func newTestLockout(store LockoutStore) *Lockout {
	return NewLockout(store, isMiss, testLockMax, testLockWindow, testLockTTL)
}

func TestLockout_NotLockedInitially(t *testing.T) {
	l := newTestLockout(newFakeStore())
	ctx := context.Background()

	locked, retryAfter := l.Locked(ctx, "alice", "10.0.0.1")
	if locked {
		t.Fatal("expected not locked initially")
	}
	if retryAfter != 0 {
		t.Fatalf("retryAfter = %v, want 0", retryAfter)
	}
}

func TestLockout_LocksAfterThreshold(t *testing.T) {
	l := newTestLockout(newFakeStore())
	ctx := context.Background()

	for i := 0; i < testLockMax; i++ {
		l.RecordFailure(ctx, "alice", "10.0.0.1")
	}

	locked, retryAfter := l.Locked(ctx, "alice", "10.0.0.1")
	if !locked {
		t.Fatal("expected locked after reaching the threshold")
	}
	if retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want > 0", retryAfter)
	}
}

func TestLockout_BelowThresholdNotLocked(t *testing.T) {
	l := newTestLockout(newFakeStore())
	ctx := context.Background()

	for i := 0; i < testLockMax-1; i++ {
		l.RecordFailure(ctx, "alice", "10.0.0.1")
	}
	if locked, _ := l.Locked(ctx, "alice", "10.0.0.1"); locked {
		t.Fatal("expected not locked below the threshold")
	}
}

func TestLockout_ResetClearsLock(t *testing.T) {
	l := newTestLockout(newFakeStore())
	ctx := context.Background()

	for i := 0; i < testLockMax; i++ {
		l.RecordFailure(ctx, "alice", "10.0.0.1")
	}
	if locked, _ := l.Locked(ctx, "alice", "10.0.0.1"); !locked {
		t.Fatal("expected locked before reset")
	}

	l.Reset(ctx, "alice", "10.0.0.1")
	if locked, _ := l.Locked(ctx, "alice", "10.0.0.1"); locked {
		t.Fatal("expected not locked after reset")
	}
}

func TestLockout_UnlockClearsUserLockOnly(t *testing.T) {
	l := newTestLockout(newFakeStore())
	ctx := context.Background()

	// Repeated failures lock both the username and the IP.
	for i := 0; i < testLockMax; i++ {
		l.RecordFailure(ctx, "alice", "10.0.0.1")
	}

	if err := l.Unlock(ctx, "alice"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	// The user-scoped lock is cleared.
	if locked, _ := l.Locked(ctx, "alice", ""); locked {
		t.Fatal("expected the user lock to be cleared after unlock")
	}
	// The IP-scoped lock is untouched: an admin unlocking an account knows the
	// user but not the originating IP.
	if locked, _ := l.Locked(ctx, "", "10.0.0.1"); !locked {
		t.Fatal("expected the IP lock to survive a user-scoped unlock")
	}
}

func TestLockout_LocksByIPAcrossUsernames(t *testing.T) {
	l := newTestLockout(newFakeStore())
	ctx := context.Background()

	// Repeated failures from one IP against different usernames trip the IP
	// lock, which then blocks even a fresh username from that IP.
	for i := 0; i < testLockMax; i++ {
		l.RecordFailure(ctx, "user"+string(rune('a'+i)), "10.0.0.9")
	}
	if locked, _ := l.Locked(ctx, "brand-new", "10.0.0.9"); !locked {
		t.Fatal("expected the IP lock to block a new username from the same IP")
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want int
	}{
		{"zero", 0, 1},
		{"negative", -5 * time.Second, 1},
		{"sub-second rounds up", 200 * time.Millisecond, 1},
		{"exact second", time.Second, 1},
		{"rounds up", 1500 * time.Millisecond, 2},
		{"minute", time.Minute, 60},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RetryAfterSeconds(tc.in); got != tc.want {
				t.Fatalf("RetryAfterSeconds(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
