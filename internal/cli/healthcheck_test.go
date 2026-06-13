package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthcheckClient_PlainHTTP(t *testing.T) {
	client, err := healthcheckClient("", "", "", 2*time.Second)
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
	if _, err := healthcheckClient(filepath.Join(t.TempDir(), "absent.pem"), "", "", time.Second); err == nil {
		t.Fatal("healthcheckClient() = nil error, want error for missing cacert")
	}
}

func TestHealthcheckClient_InvalidPEM(t *testing.T) {
	f := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(f, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write cacert: %v", err)
	}
	if _, err := healthcheckClient(f, "", "", time.Second); err == nil {
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

	client, err := healthcheckClient(caFile, "", "", 5*time.Second)
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
	untrusted, err := healthcheckClient("", "", "", time.Second)
	if err != nil {
		t.Fatalf("healthcheckClient() error: %v", err)
	}
	if _, err := untrusted.Get(srv.URL); err == nil {
		t.Error("client.Get() without CA = nil error, want TLS verification failure")
	}
}

func TestHealthcheckClient_CertWithoutKey(t *testing.T) {
	if _, err := healthcheckClient("", "cert.pem", "", time.Second); err == nil {
		t.Fatal("healthcheckClient() = nil error, want error for --cert without --key")
	}
	if _, err := healthcheckClient("", "", "key.pem", time.Second); err == nil {
		t.Fatal("healthcheckClient() = nil error, want error for --key without --cert")
	}
}

func TestHealthcheckClient_BadClientCertPath(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "absent-cert.pem")
	key := filepath.Join(dir, "absent-key.pem")
	if _, err := healthcheckClient("", cert, key, time.Second); err == nil {
		t.Fatal("healthcheckClient() = nil error, want error for unreadable client cert")
	}
}

// TestHealthcheckClient_MutualTLS spins up a TLS server that requires and
// verifies a client certificate, and confirms the probe succeeds only when it
// presents one signed by the server's trusted CA.
func TestHealthcheckClient_MutualTLS(t *testing.T) {
	certPEM, keyPEM := selfSignedPair(t)

	keyPair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build key pair: %v", err)
	}
	leaf, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	defer srv.Close()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	certFile := filepath.Join(dir, "client.pem")
	keyFile := filepath.Join(dir, "client-key.pem")
	writes := map[string][]byte{caFile: certPEM, certFile: certPEM, keyFile: keyPEM}
	for f, data := range writes {
		if err := os.WriteFile(f, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}

	// Without a client certificate the handshake must be rejected.
	noCert, err := healthcheckClient(caFile, "", "", 5*time.Second)
	if err != nil {
		t.Fatalf("healthcheckClient() error: %v", err)
	}
	if _, err := noCert.Get(srv.URL); err == nil {
		t.Error("client.Get() without client cert = nil error, want mTLS rejection")
	}

	// Presenting the trusted client certificate succeeds.
	withCert, err := healthcheckClient(caFile, certFile, keyFile, 5*time.Second)
	if err != nil {
		t.Fatalf("healthcheckClient() error: %v", err)
	}
	resp, err := withCert.Get(srv.URL)
	if err != nil {
		t.Fatalf("client.Get() with client cert: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// selfSignedPair returns a PEM certificate/key usable for both server and
// client authentication (loopback SANs), suitable for exercising mutual TLS.
func selfSignedPair(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM
}
