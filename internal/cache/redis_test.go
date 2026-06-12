package cache_test

import (
	"testing"

	"github.com/vancanhuit/devopsbin/internal/cache"
	"github.com/vancanhuit/devopsbin/internal/config"
)

// These tests exercise the infra-free paths of the cache client: constructor
// validation and Close safety. They carry no build tag so they run as part of
// the regular unit suite without requiring a live Redis.

func standaloneCfg() config.RedisConfig {
	return config.RedisConfig{Mode: config.RedisStandalone, Addrs: []string{"localhost:6379"}}
}

func TestClient_New_InvalidConfig_Unit(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.RedisConfig
	}{
		{"empty mode", config.RedisConfig{Addrs: []string{"localhost:6379"}}},
		{"unknown mode", config.RedisConfig{Mode: "proxy", Addrs: []string{"localhost:6379"}}},
		{"no addrs", config.RedisConfig{Mode: config.RedisStandalone}},
		{"standalone multiple addrs", config.RedisConfig{Mode: config.RedisStandalone, Addrs: []string{"a:6379", "b:6379"}}},
		{"sentinel without master", config.RedisConfig{Mode: config.RedisSentinel, Addrs: []string{"s:26379"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := cache.New(tc.cfg); err == nil {
				t.Fatalf("New(%+v) = nil error, want error", tc.cfg)
			}
		})
	}
}

func TestClient_New_ValidModes_Unit(t *testing.T) {
	// New does not connect, so every valid topology constructs without a live
	// Redis. This verifies mode selection does not error on the happy paths.
	tests := []struct {
		name string
		cfg  config.RedisConfig
	}{
		{"standalone", standaloneCfg()},
		{"cluster", config.RedisConfig{Mode: config.RedisCluster, Addrs: []string{"n1:6379", "n2:6379", "n3:6379"}}},
		{"sentinel", config.RedisConfig{Mode: config.RedisSentinel, Addrs: []string{"s1:26379", "s2:26379"}, MasterName: "mymaster"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := cache.New(tc.cfg)
			if err != nil {
				t.Fatalf("New(%s) = %v, want nil", tc.name, err)
			}
			t.Cleanup(func() { _ = c.Close() })
		})
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
	c, err := cache.New(standaloneCfg())
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close = %v, want nil", err)
	}
}
