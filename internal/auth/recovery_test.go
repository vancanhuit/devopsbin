package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecovery_TokenRoundtrip(t *testing.T) {
	store := newFakeStore()
	r := NewRecovery(store, isMiss, 15*time.Minute)
	ctx := context.Background()

	token, err := r.CreateResetToken(ctx, "user-1")
	if err != nil {
		t.Fatalf("CreateResetToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty token")
	}

	userID, err := r.ConsumeResetToken(ctx, token)
	if err != nil {
		t.Fatalf("ConsumeResetToken: %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("userID = %q, want %q", userID, "user-1")
	}
}

func TestRecovery_TokenIsSingleUse(t *testing.T) {
	store := newFakeStore()
	r := NewRecovery(store, isMiss, 15*time.Minute)
	ctx := context.Background()

	token, err := r.CreateResetToken(ctx, "user-1")
	if err != nil {
		t.Fatalf("CreateResetToken: %v", err)
	}
	if _, err := r.ConsumeResetToken(ctx, token); err != nil {
		t.Fatalf("first consume: %v", err)
	}

	// A second consume of the same token must fail.
	if _, err := r.ConsumeResetToken(ctx, token); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("second consume err = %v, want ErrResetTokenInvalid", err)
	}
}

func TestRecovery_ConsumeUnknownToken(t *testing.T) {
	store := newFakeStore()
	r := NewRecovery(store, isMiss, 15*time.Minute)
	ctx := context.Background()

	if _, err := r.ConsumeResetToken(ctx, "does-not-exist"); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("err = %v, want ErrResetTokenInvalid", err)
	}
}

func TestRecovery_ConsumeEmptyToken(t *testing.T) {
	store := newFakeStore()
	r := NewRecovery(store, isMiss, 15*time.Minute)
	ctx := context.Background()

	if _, err := r.ConsumeResetToken(ctx, ""); !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("err = %v, want ErrResetTokenInvalid", err)
	}
}

func TestRecovery_TokenStoredWithTTL(t *testing.T) {
	store := newFakeStore()
	const ttl = 15 * time.Minute
	r := NewRecovery(store, isMiss, ttl)
	ctx := context.Background()

	token, err := r.CreateResetToken(ctx, "user-1")
	if err != nil {
		t.Fatalf("CreateResetToken: %v", err)
	}
	got, ok := store.ttl(resetKey(token))
	if !ok {
		t.Fatal("expected the reset token to be stored with a TTL")
	}
	if got != ttl {
		t.Fatalf("ttl = %v, want %v", got, ttl)
	}
}
