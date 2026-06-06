package cache_test

import (
	"testing"

	"github.com/vancanhuit/devopsbin/internal/cache"
)

// These tests exercise the infra-free paths of the cache client: constructor
// validation and Close safety. They carry no build tag so they run as part of
// the regular unit suite without requiring a live Redis.

func TestClient_New_EmptyURL_Unit(t *testing.T) {
	if _, err := cache.New(""); err == nil {
		t.Fatal("expected error for empty url, got nil")
	}
}

func TestClient_New_InvalidURL_Unit(t *testing.T) {
	if _, err := cache.New("://not-a-url"); err == nil {
		t.Fatal("expected error for malformed url, got nil")
	}
}

func TestClient_Close_NilReceiver(t *testing.T) {
	var c *cache.Client
	// Must not panic and must return nil on a nil receiver.
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil receiver = %v, want nil", err)
	}
}

func TestClient_Close_AfterNew(t *testing.T) {
	// New does not connect, so this validates Close on a never-pinged client.
	c, err := cache.New("redis://localhost:6379/0")
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}
}
