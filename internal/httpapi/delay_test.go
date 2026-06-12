package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

func TestGetDelay_OK(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/delay/0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.DelayResponse](t, rec)
	if body.Delay != 0 {
		t.Errorf("delay: got %v, want 0", body.Delay)
	}
}

func TestGetDelay_SmallDelay(t *testing.T) {
	h := httpapi.NewServer().Handler()

	start := time.Now()
	rec := doGet(t, h, "/api/v1/delay/0.05")
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("elapsed: got %v, want >= 50ms", elapsed)
	}
	body := decode[httpapi.DelayResponse](t, rec)
	if body.Delay != 0.05 {
		t.Errorf("delay: got %v, want 0.05", body.Delay)
	}
}

func TestGetDelay_Negative(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/delay/-1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
	body := decode[httpapi.ErrorResponse](t, rec)
	if body.Error == "" {
		t.Error("error body empty, want a message")
	}
}

func TestGetDelay_NonNumeric(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/delay/abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestGetDelay_RespectsCancellation verifies the handler returns promptly when
// the request context is cancelled rather than sleeping the full duration.
func TestGetDelay_RespectsCancellation(t *testing.T) {
	srv := httpapi.NewServer()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	resp, err := srv.GetDelay(ctx, httpapi.GetDelayRequestObject{Seconds: 5})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("got nil error, want context cancellation")
	}
	if resp != nil {
		t.Errorf("response: got %v, want nil", resp)
	}
	if elapsed > time.Second {
		t.Errorf("elapsed: got %v, want prompt return", elapsed)
	}
}
