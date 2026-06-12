//go:build integration

// End-to-end integration tests for the readiness and startup probes. These
// wire the real Postgres store and Redis cache into the server and exercise the
// probes through the chi router. Run via:
//
//	mise run api:test:integration
//
// Requires live dependencies via the loaded config (POSTGRES_URL,
// REDIS_MODE/REDIS_ADDRS), which mise.test.toml (MISE_ENV=test) points at the
// test-profile instances.

package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/cache"
	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/httpapi"
	"github.com/vancanhuit/devopsbin/internal/store"
)

// realDeps connects to the test-profile Postgres and Redis, returning a server
// configured with readiness and startup checks against them. Connection details
// come from the same config package used in production.
func realDeps(t *testing.T) http.Handler {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	db, err := store.New(ctx, cfg.Postgres.URL)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(db.Close)

	rdb, err := cache.New(cfg.Redis)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	const timeout = 2 * time.Second
	srv := httpapi.NewServer(
		httpapi.WithReadinessCheck("postgres", httpapi.PingCheck(db, timeout)),
		httpapi.WithReadinessCheck("redis", httpapi.PingCheck(rdb, timeout)),
		httpapi.WithStartupCheck("postgres", httpapi.PingCheck(db, timeout)),
		httpapi.WithStartupCheck("redis", httpapi.PingCheck(rdb, timeout)),
	)
	return srv.Handler()
}

func TestReadyz_HealthyDependencies(t *testing.T) {
	h := realDeps(t)

	rec := doGet(t, h, "/api/v1/readyz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decode[httpapi.ReadyzResponse](t, rec)
	if got.Status != httpapi.Ready {
		t.Fatalf("status = %q, want %q", got.Status, httpapi.Ready)
	}
	for _, name := range []string{"postgres", "redis"} {
		c, ok := got.Checks[name]
		if !ok {
			t.Fatalf("missing %q check in %v", name, got.Checks)
		}
		if c.Status != httpapi.DependencyCheckStatusOk {
			t.Fatalf("%s status = %q, want %q", name, c.Status, httpapi.DependencyCheckStatusOk)
		}
	}
}

func TestStartupz_HealthyDependencies(t *testing.T) {
	h := realDeps(t)

	rec := doGet(t, h, "/api/v1/startupz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	got := decode[httpapi.StartupzResponse](t, rec)
	if got.Status != httpapi.Started {
		t.Fatalf("status = %q, want %q", got.Status, httpapi.Started)
	}
}

func TestLivez_HealthyDependencies(t *testing.T) {
	h := realDeps(t)

	rec := doGet(t, h, "/api/v1/livez")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
