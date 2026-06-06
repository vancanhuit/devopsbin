package store_test

import (
	"context"
	"testing"

	"github.com/vancanhuit/devopsbin/internal/store"
)

// These tests exercise the infra-free paths of the store: constructor
// validation and Close safety. They carry no build tag so they run as part of
// the regular unit suite without requiring a live Postgres.

func TestStore_New_EmptyURL_Unit(t *testing.T) {
	if _, err := store.New(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty url, got nil")
	}
}

func TestStore_New_InvalidURL_Unit(t *testing.T) {
	if _, err := store.New(context.Background(), "://not-a-url"); err == nil {
		t.Fatal("expected error for malformed url, got nil")
	}
}

func TestStore_Close_NilReceiver(t *testing.T) {
	var s *store.Store
	// Must not panic on a nil receiver.
	s.Close()
}

func TestStore_Close_AfterNew(t *testing.T) {
	// New does not connect, so this validates Close on a never-pinged pool.
	s, err := store.New(context.Background(), "postgres://user:pass@localhost:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	s.Close()
}
