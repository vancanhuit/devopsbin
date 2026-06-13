package cli

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthcheckClient_PlainHTTP(t *testing.T) {
	client, err := healthcheckClient("", 2*time.Second)
	if err != nil {
		t.Fatalf("healthcheckClient() error: %v", err)
	}
	if client.Timeout != 2*time.Second {
		t.Errorf("Timeout = %v, want 2s", client.Timeout)
	}
	// No custom transport is needed for plain HTTP.
	if client.Transport != nil {
		t.Errorf("Transport = %v, want nil for plain HTTP", client.Transport)
	}
}

func TestHealthcheckClient_BadCACertPath(t *testing.T) {
	if _, err := healthcheckClient(filepath.Join(t.TempDir(), "absent.pem"), time.Second); err == nil {
		t.Fatal("healthcheckClient() = nil error, want error for missing cacert")
	}
}

func TestHealthcheckClient_InvalidPEM(t *testing.T) {
	f := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(f, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write cacert: %v", err)
	}
	if _, err := healthcheckClient(f, time.Second); err == nil {
		t.Fatal("healthcheckClient() = nil error, want error for invalid PEM")
	}
}

// TestHealthcheckClient_VerifiesAgainstCA spins up an in-process TLS server and
// confirms the probe client trusts it only when handed the server's CA PEM.
func TestHealthcheckClient_VerifiesAgainstCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("write cacert: %v", err)
	}

	client, err := healthcheckClient(caFile, 5*time.Second)
	if err != nil {
		t.Fatalf("healthcheckClient() error: %v", err)
	}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("client.Get() with trusted CA: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	// A client without the CA must reject the self-signed server.
	untrusted, err := healthcheckClient("", time.Second)
	if err != nil {
		t.Fatalf("healthcheckClient() error: %v", err)
	}
	if _, err := untrusted.Get(srv.URL); err == nil {
		t.Error("client.Get() without CA = nil error, want TLS verification failure")
	}
}
