package auth

import (
	"context"
	"time"
)

// Lockout keys namespace per-user and per-IP failure counters and locks.
const (
	loginFailUserKeyPrefix = "loginfail:v1:user:"
	loginFailIPKeyPrefix   = "loginfail:v1:ip:"
	loginLockUserKeyPrefix = "lock:login:v1:user:"
	loginLockIPKeyPrefix   = "lock:login:v1:ip:"
)

// LockoutStore is the subset of store operations the Lockout type needs. A
// cache client (Redis) satisfies it. Incr increments a counter and sets its
// window TTL on first use; Get must surface a miss recognizable by isMiss.
type LockoutStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Del(ctx context.Context, key string) error
}

// Lockout throttles password-guessing by counting recent failed logins per
// username and per client IP within a fixed window and locking further attempts
// once a threshold is crossed. It is a cache-backed best-effort defense: if the
// store is unavailable it fails open (never blocks a legitimate login) rather
// than locking everyone out.
type Lockout struct {
	store       LockoutStore
	isMiss      func(error) bool
	maxAttempts int
	window      time.Duration
	lockTTL     time.Duration
}

// NewLockout builds a Lockout. maxAttempts is the number of failures within
// window that triggers a lock; lockTTL is how long a lock lasts.
func NewLockout(store LockoutStore, isMiss func(error) bool, maxAttempts int, window, lockTTL time.Duration) *Lockout {
	return &Lockout{
		store:       store,
		isMiss:      isMiss,
		maxAttempts: maxAttempts,
		window:      window,
		lockTTL:     lockTTL,
	}
}

// Locked reports whether logins for username or ip are currently locked and, if
// so, how long the caller should wait before retrying (the longer of the two
// remaining lock TTLs). A store error fails open: it returns not-locked so a
// cache outage never blocks legitimate users.
func (l *Lockout) Locked(ctx context.Context, username, ip string) (bool, time.Duration) {
	var retryAfter time.Duration
	locked := false
	for _, key := range l.lockKeys(username, ip) {
		_, err := l.store.Get(ctx, key)
		if err != nil {
			if l.isMiss(err) {
				continue
			}
			// Fail open on a real store error.
			return false, 0
		}
		locked = true
		if ttl, err := l.store.TTL(ctx, key); err == nil && ttl > retryAfter {
			retryAfter = ttl
		}
	}
	if locked && retryAfter <= 0 {
		// A lock with no readable TTL still warrants a sensible hint.
		retryAfter = l.lockTTL
	}
	return locked, retryAfter
}

// RecordFailure counts a failed login against both username and ip. When either
// counter reaches maxAttempts within the window, the corresponding key is
// locked for lockTTL. Store errors are ignored so a cache outage degrades to no
// throttling rather than breaking the login path.
func (l *Lockout) RecordFailure(ctx context.Context, username, ip string) {
	l.bump(ctx, loginFailUserKeyPrefix+username, loginLockUserKeyPrefix+username)
	l.bump(ctx, loginFailIPKeyPrefix+ip, loginLockIPKeyPrefix+ip)
}

func (l *Lockout) bump(ctx context.Context, counterKey, lockKey string) {
	n, err := l.store.Incr(ctx, counterKey, l.window)
	if err != nil {
		return
	}
	if int(n) >= l.maxAttempts {
		_ = l.store.Set(ctx, lockKey, "1", l.lockTTL)
	}
}

// Reset clears the failure counters and locks for username and ip after a
// successful login. Store errors are ignored.
func (l *Lockout) Reset(ctx context.Context, username, ip string) {
	keys := []string{
		loginFailUserKeyPrefix + username,
		loginFailIPKeyPrefix + ip,
		loginLockUserKeyPrefix + username,
		loginLockIPKeyPrefix + ip,
	}
	for _, key := range keys {
		_ = l.store.Del(ctx, key)
	}
}

func (l *Lockout) lockKeys(username, ip string) []string {
	keys := make([]string, 0, 2)
	if username != "" {
		keys = append(keys, loginLockUserKeyPrefix+username)
	}
	if ip != "" {
		keys = append(keys, loginLockIPKeyPrefix+ip)
	}
	return keys
}

// RetryAfterSeconds converts a retry-after duration to whole seconds, rounding
// up and reporting at least one second so a client never busy-retries.
func RetryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return secs
}
