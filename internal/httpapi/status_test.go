package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

func TestGetStatus_OK(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/status/200")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.StatusResponse](t, rec)
	if body.Code != 200 {
		t.Errorf("body code: got %d, want 200", body.Code)
	}
	if body.Description == nil || *body.Description != "OK" {
		t.Errorf("body description: got %v, want \"OK\"", body.Description)
	}
}

func TestGetStatus_ArbitraryCode(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/status/503")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	body := decode[httpapi.StatusResponse](t, rec)
	if body.Code != 503 {
		t.Errorf("body code: got %d, want 503", body.Code)
	}
	if body.Description == nil || *body.Description != "Service Unavailable" {
		t.Errorf("body description: got %v, want \"Service Unavailable\"", body.Description)
	}
}

func TestGetStatus_NoBodyCodes(t *testing.T) {
	h := httpapi.NewServer().Handler()

	for _, code := range []string{"204", "304", "100"} {
		rec := doGet(t, h, "/api/v1/status/"+code)
		if got := rec.Body.Len(); got != 0 {
			t.Errorf("status/%s: body length %d, want 0", code, got)
		}
	}
}

func TestGetStatus_UnknownCodeNoDescription(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/status/599")
	if rec.Code != 599 {
		t.Fatalf("status: got %d, want 599", rec.Code)
	}
	body := decode[httpapi.StatusResponse](t, rec)
	if body.Code != 599 {
		t.Errorf("body code: got %d, want 599", body.Code)
	}
	if body.Description != nil {
		t.Errorf("body description: got %v, want nil", body.Description)
	}
}

func TestGetStatus_OutOfRange(t *testing.T) {
	h := httpapi.NewServer().Handler()

	for _, code := range []string{"99", "600"} {
		rec := doGet(t, h, "/api/v1/status/"+code)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status/%s: got %d, want %d", code, rec.Code, http.StatusBadRequest)
		}
		body := decode[httpapi.ErrorResponse](t, rec)
		if body.Error == "" {
			t.Errorf("status/%s: error body empty, want a message", code)
		}
	}
}

func TestGetStatus_NonInteger(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/status/abc")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
