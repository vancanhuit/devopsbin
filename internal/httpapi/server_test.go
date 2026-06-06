package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

// doGet issues a GET request against h and returns the recorded response.
func doGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decode unmarshals the recorder body into v, failing the test on error.
func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return v
}

func okCheck(context.Context) httpapi.DependencyCheck {
	return httpapi.DependencyCheck{Status: httpapi.DependencyCheckStatusOk}
}

func errCheck(context.Context) httpapi.DependencyCheck {
	msg := "connection refused"
	return httpapi.DependencyCheck{Status: httpapi.DependencyCheckStatusError, Message: &msg}
}

func skippedCheck(context.Context) httpapi.DependencyCheck {
	return httpapi.DependencyCheck{Status: httpapi.DependencyCheckStatusSkipped}
}

func TestGetLivez(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/livez")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := decode[httpapi.LivezResponse](t, rec)
	if body.Status != httpapi.LivezResponseStatusOk {
		t.Errorf("status = %q, want %q", body.Status, httpapi.LivezResponseStatusOk)
	}
}

func TestGetReadyz_AllHealthy(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithReadinessCheck("postgres", okCheck),
		httpapi.WithReadinessCheck("redis", okCheck),
	).Handler()

	rec := doGet(t, h, "/api/v1/readyz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.ReadyzResponse](t, rec)
	if body.Status != httpapi.Ready {
		t.Errorf("status = %q, want %q", body.Status, httpapi.Ready)
	}
	if len(body.Checks) != 2 {
		t.Errorf("checks len = %d, want 2", len(body.Checks))
	}
	for name, c := range body.Checks {
		if c.Status != httpapi.DependencyCheckStatusOk {
			t.Errorf("check %q status = %q, want ok", name, c.Status)
		}
	}
}

func TestGetReadyz_NoChecks(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/readyz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no checks means ready)", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.ReadyzResponse](t, rec)
	if body.Status != httpapi.Ready {
		t.Errorf("status = %q, want %q", body.Status, httpapi.Ready)
	}
	if len(body.Checks) != 0 {
		t.Errorf("checks len = %d, want 0", len(body.Checks))
	}
}

func TestGetReadyz_Unhealthy(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithReadinessCheck("postgres", okCheck),
		httpapi.WithReadinessCheck("redis", errCheck),
	).Handler()

	rec := doGet(t, h, "/api/v1/readyz")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	body := decode[httpapi.ReadyzResponse](t, rec)
	if body.Status != httpapi.NotReady {
		t.Errorf("status = %q, want %q", body.Status, httpapi.NotReady)
	}
	redis := body.Checks["redis"]
	if redis.Status != httpapi.DependencyCheckStatusError {
		t.Errorf("redis status = %q, want error", redis.Status)
	}
	if redis.Message == nil || *redis.Message != "connection refused" {
		t.Errorf("redis message = %v, want %q", redis.Message, "connection refused")
	}
}

func TestGetReadyz_SkippedIsHealthy(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithReadinessCheck("migrations", skippedCheck),
	).Handler()

	rec := doGet(t, h, "/api/v1/readyz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (skipped must not fail readiness)", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.ReadyzResponse](t, rec)
	if body.Status != httpapi.Ready {
		t.Errorf("status = %q, want %q", body.Status, httpapi.Ready)
	}
}

func TestGetStartupz_Started(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithStartupCheck("config", okCheck),
		httpapi.WithStartupCheck("migrations", okCheck),
	).Handler()

	rec := doGet(t, h, "/api/v1/startupz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.StartupzResponse](t, rec)
	if body.Status != httpapi.Started {
		t.Errorf("status = %q, want %q", body.Status, httpapi.Started)
	}
}

func TestGetStartupz_Starting(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithStartupCheck("migrations", errCheck),
	).Handler()

	rec := doGet(t, h, "/api/v1/startupz")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	body := decode[httpapi.StartupzResponse](t, rec)
	if body.Status != httpapi.Starting {
		t.Errorf("status = %q, want %q", body.Status, httpapi.Starting)
	}
}

func TestGetStartupz_SkippedIsStarted(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithStartupCheck("migrations", skippedCheck),
	).Handler()

	rec := doGet(t, h, "/api/v1/startupz")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (skipped must not block startup)", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.StartupzResponse](t, rec)
	if body.Status != httpapi.Started {
		t.Errorf("status = %q, want %q", body.Status, httpapi.Started)
	}
}

func TestGetStartupz_MixedFailsStarting(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithStartupCheck("postgres", okCheck),
		httpapi.WithStartupCheck("redis", errCheck),
	).Handler()

	rec := doGet(t, h, "/api/v1/startupz")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	body := decode[httpapi.StartupzResponse](t, rec)
	if body.Status != httpapi.Starting {
		t.Errorf("status = %q, want %q", body.Status, httpapi.Starting)
	}
	if redis := body.Checks["redis"]; redis.Status != httpapi.DependencyCheckStatusError {
		t.Errorf("redis status = %q, want error", redis.Status)
	}
	if pg := body.Checks["postgres"]; pg.Status != httpapi.DependencyCheckStatusOk {
		t.Errorf("postgres status = %q, want ok", pg.Status)
	}
}

func TestGetVersion(t *testing.T) {
	buildTime := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	h := httpapi.NewServer(httpapi.WithBuildInfo(httpapi.BuildInfo{
		Service:   "devopsbin-api",
		Version:   "1.2.3",
		GitSHA:    "abc123",
		BuildTime: buildTime,
		GoVersion: runtime.Version(),
	})).Handler()

	rec := doGet(t, h, "/api/v1/version")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.VersionResponse](t, rec)
	want := httpapi.VersionResponse{
		Service:   "devopsbin-api",
		Version:   "1.2.3",
		GitSha:    "abc123",
		BuildTime: buildTime,
		GoVersion: runtime.Version(),
	}
	if body != want {
		t.Errorf("body = %+v\nwant %+v", body, want)
	}
}

func TestGetVersion_DefaultsGoVersion(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/version")

	body := decode[httpapi.VersionResponse](t, rec)
	if body.GoVersion == "" {
		t.Error("GoVersion is empty, want runtime.Version() default")
	}
}

func TestUnknownRoute(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
