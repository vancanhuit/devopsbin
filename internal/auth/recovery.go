package auth

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// resetKeyPrefix namespaces single-use password-reset tokens in the store.
const resetKeyPrefix = "pwreset:v1:"

// ErrResetTokenInvalid indicates a reset token is unknown, expired, or already
// consumed. The three cases are intentionally indistinguishable so a caller
// cannot probe which tokens exist.
var ErrResetTokenInvalid = errors.New("auth: password reset token invalid or expired")

// ResetStore is the subset of store operations the Recovery type needs. A cache
// client (Redis) satisfies it. GetDel must atomically return and delete the
// key, and surface a miss recognizable by the isMiss predicate.
type ResetStore interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	GetDel(ctx context.Context, key string) (string, error)
}

// Recovery mints and consumes single-use, expiring password-reset tokens backed
// by a TTL store. Tokens are opaque random strings mapped to a user id; they
// are consumed atomically so a token works at most once.
type Recovery struct {
	store  ResetStore
	isMiss func(error) bool
	ttl    time.Duration
}

// NewRecovery builds a Recovery. isMiss reports whether a store error is a key
// miss (versus a real failure); ttl is the token lifetime.
func NewRecovery(store ResetStore, isMiss func(error) bool, ttl time.Duration) *Recovery {
	return &Recovery{store: store, isMiss: isMiss, ttl: ttl}
}

// CreateResetToken mints a token bound to userID and stores it with the
// configured TTL. The returned token is delivered to the user (emailed in
// production; returned in the response for this demo).
func (r *Recovery) CreateResetToken(ctx context.Context, userID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := r.store.Set(ctx, resetKey(token), userID, r.ttl); err != nil {
		return "", fmt.Errorf("auth: store reset token: %w", err)
	}
	return token, nil
}

// ConsumeResetToken atomically validates and consumes token, returning the
// bound user id. A miss (unknown, expired, or already-used token) returns
// ErrResetTokenInvalid.
func (r *Recovery) ConsumeResetToken(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrResetTokenInvalid
	}
	userID, err := r.store.GetDel(ctx, resetKey(token))
	if err != nil {
		if r.isMiss(err) {
			return "", ErrResetTokenInvalid
		}
		return "", fmt.Errorf("auth: consume reset token: %w", err)
	}
	return userID, nil
}

func resetKey(token string) string {
	return resetKeyPrefix + token
}
