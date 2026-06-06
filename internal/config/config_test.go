package config_test

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/config"
)

// envKeys are every environment variable Load reads. clearEnv unsets all of
// them so each test starts from a known, default state regardless of the
// ambient environment.
var envKeys = []string{
	"APP_ENV",
	"APP_VERSION",
	"APP_GIT_SHA",
	"APP_BUILD_TIME",
	"APP_LOG_LEVEL",
	"HTTP_ADDR",
	"HTTP_READ_TIMEOUT",
	"HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT",
	"HTTP_SHUTDOWN_TIMEOUT",
	"POSTGRES_URL",
	"REDIS_URL",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range envKeys {
		// t.Setenv records the original value for restoration after the
		// test; the immediate Unsetenv then clears it so envDefault values
		// apply during the test run.
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	if cfg.App.Environment != config.EnvProd {
		t.Errorf("App.Environment = %q, want %q", cfg.App.Environment, config.EnvProd)
	}
	if cfg.App.Version != "dev" {
		t.Errorf("App.Version = %q, want dev", cfg.App.Version)
	}
	if cfg.App.LogLevel != "info" {
		t.Errorf("App.LogLevel = %q, want info", cfg.App.LogLevel)
	}
	if cfg.Http.Addr != ":8080" {
		t.Errorf("Http.Addr = %q, want :8080", cfg.Http.Addr)
	}
	if cfg.Http.ReadTimeout != 5*time.Second {
		t.Errorf("Http.ReadTimeout = %v, want 5s", cfg.Http.ReadTimeout)
	}
	if cfg.Http.WriteTimeout != 10*time.Second {
		t.Errorf("Http.WriteTimeout = %v, want 10s", cfg.Http.WriteTimeout)
	}
	if cfg.Http.IdleTimeout != 60*time.Second {
		t.Errorf("Http.IdleTimeout = %v, want 60s", cfg.Http.IdleTimeout)
	}
	if cfg.Http.ShutdownTimeout != 15*time.Second {
		t.Errorf("Http.ShutdownTimeout = %v, want 15s", cfg.Http.ShutdownTimeout)
	}
	if cfg.Postgres.URL != "postgres://user:password@localhost:5432/dbname?sslmode=disable" {
		t.Errorf("Postgres.URL = %q, want default DSN", cfg.Postgres.URL)
	}
	if cfg.Redis.URL != "redis://localhost:6379/0" {
		t.Errorf("Redis.URL = %q, want default DSN", cfg.Redis.URL)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_VERSION", "1.2.3")
	t.Setenv("APP_GIT_SHA", "abc123")
	t.Setenv("APP_BUILD_TIME", "2026-06-06T10:00:00Z")
	t.Setenv("APP_LOG_LEVEL", "debug")
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("HTTP_READ_TIMEOUT", "1s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "2s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "3s")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "4s")
	t.Setenv("POSTGRES_URL", "postgres://u:p@h:5432/db?sslmode=require")
	t.Setenv("REDIS_URL", "redis://h:6379/0")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	want := config.Config{
		App: config.AppConfig{
			Environment: "dev",
			Version:     "1.2.3",
			GitSHA:      "abc123",
			BuildTime:   "2026-06-06T10:00:00Z",
			LogLevel:    "debug",
		},
		Http: config.HttpConfig{
			Addr:            ":9000",
			ReadTimeout:     1 * time.Second,
			WriteTimeout:    2 * time.Second,
			IdleTimeout:     3 * time.Second,
			ShutdownTimeout: 4 * time.Second,
		},
		Postgres: config.PostgresConfig{URL: "postgres://u:p@h:5432/db?sslmode=require"},
		Redis:    config.RedisConfig{URL: "redis://h:6379/0"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("cfg = %+v\nwant %+v", cfg, want)
	}
}

func TestLoad_InvalidEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "staging")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load(): expected error for invalid env, got nil")
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_LOG_LEVEL", "verbose")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load(): expected error for invalid log level, got nil")
	}
}

func TestValidate(t *testing.T) {
	base := func() config.Config {
		return config.Config{
			App:  config.AppConfig{Environment: config.EnvProd, LogLevel: "info"},
			Http: config.HttpConfig{Addr: ":8080", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second},
		}
	}

	tests := []struct {
		name    string
		mutate  func(c *config.Config)
		wantErr bool
	}{
		{"valid", func(*config.Config) {}, false},
		{"empty addr", func(c *config.Config) { c.Http.Addr = "" }, true},
		{"zero read timeout", func(c *config.Config) { c.Http.ReadTimeout = 0 }, true},
		{"zero write timeout", func(c *config.Config) { c.Http.WriteTimeout = 0 }, true},
		{"zero idle timeout", func(c *config.Config) { c.Http.IdleTimeout = 0 }, true},
		{"zero shutdown timeout", func(c *config.Config) { c.Http.ShutdownTimeout = 0 }, true},
		{"bad env", func(c *config.Config) { c.App.Environment = "qa" }, true},
		{"bad log level", func(c *config.Config) { c.App.LogLevel = "trace" }, true},
		{"invalid postgres url", func(c *config.Config) { c.Postgres.URL = "://bad" }, true},
		{"invalid redis url", func(c *config.Config) { c.Redis.URL = "://bad" }, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestRedacted(t *testing.T) {
	c := config.Config{
		Postgres: config.PostgresConfig{URL: "postgres://user:secret@host:5432/db"},
		Redis:    config.RedisConfig{URL: "redis://user:topsecret@host:6379/0"},
	}

	got := c.Redacted()

	if got.Postgres.URL != "postgres://user:REDACTED@host:5432/db" {
		t.Errorf("Postgres.URL = %q, want password redacted", got.Postgres.URL)
	}
	if got.Redis.URL != "redis://user:REDACTED@host:6379/0" {
		t.Errorf("Redis.URL = %q, want password redacted", got.Redis.URL)
	}

	// The original must be untouched.
	if c.Postgres.URL != "postgres://user:secret@host:5432/db" {
		t.Errorf("Redacted mutated the receiver: %q", c.Postgres.URL)
	}
}

func TestRedacted_NoCredentials(t *testing.T) {
	c := config.Config{
		Postgres: config.PostgresConfig{URL: "postgres://host:5432/db"},
		Redis:    config.RedisConfig{URL: ""},
	}

	got := c.Redacted()

	if got.Postgres.URL != "postgres://host:5432/db" {
		t.Errorf("Postgres.URL = %q, want unchanged", got.Postgres.URL)
	}
	if got.Redis.URL != "" {
		t.Errorf("Redis.URL = %q, want empty", got.Redis.URL)
	}
}
