package httpapi_test

import (
	"net/http"
	"net/netip"
	"testing"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

func mustPrefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			t.Fatalf("parse prefix %q: %v", c, err)
		}
		prefixes = append(prefixes, p)
	}
	return prefixes
}

func originFor(t *testing.T, h http.Handler, remoteAddr string, xff string) string {
	t.Helper()
	rec := doGetWith(t, h, "/api/v1/ip", func(r *http.Request) {
		r.RemoteAddr = remoteAddr
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	return decode[httpapi.IpResponse](t, rec).Origin
}

func TestTrustedProxy_HonorsXFFFromTrustedPeer(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithTrustedProxies(mustPrefixes(t, "10.0.0.0/8")),
	).Handler()

	got := originFor(t, h, "10.1.2.3:443", "203.0.113.7")
	if got != "203.0.113.7" {
		t.Errorf("origin = %q, want %q (client behind trusted proxy)", got, "203.0.113.7")
	}
}

func TestTrustedProxy_IgnoresXFFFromUntrustedPeer(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithTrustedProxies(mustPrefixes(t, "10.0.0.0/8")),
	).Handler()

	// Peer is not in a trusted range; the spoofed header must be ignored and
	// the peer address used instead.
	got := originFor(t, h, "203.0.113.42:54321", "10.9.9.9")
	if got != "203.0.113.42" {
		t.Errorf("origin = %q, want %q (spoofed XFF ignored)", got, "203.0.113.42")
	}
}

func TestTrustedProxy_MultiHopSelectsRightmostUntrusted(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithTrustedProxies(mustPrefixes(t, "10.0.0.0/8")),
	).Handler()

	// Chain: real client -> outer proxy -> inner proxy (peer). Walking
	// right-to-left we skip the trusted proxy hops and stop at the client.
	got := originFor(t, h, "10.0.0.2:443", "203.0.113.7, 10.0.0.1")
	if got != "203.0.113.7" {
		t.Errorf("origin = %q, want %q (rightmost untrusted)", got, "203.0.113.7")
	}
}

func TestTrustedProxy_NoPrefixesIgnoresXFF(t *testing.T) {
	h := httpapi.NewServer().Handler()

	got := originFor(t, h, "203.0.113.42:54321", "10.9.9.9")
	if got != "203.0.113.42" {
		t.Errorf("origin = %q, want %q (no trusted proxies configured)", got, "203.0.113.42")
	}
}

func schemeFor(t *testing.T, h http.Handler, remoteAddr string, xfp string) string {
	t.Helper()
	rec := doGetWith(t, h, "/api/v1/scheme", func(r *http.Request) {
		r.RemoteAddr = remoteAddr
		if xfp != "" {
			r.Header.Set("X-Forwarded-Proto", xfp)
		}
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	return string(decode[httpapi.SchemeResponse](t, rec).Scheme)
}

func TestTrustedProxy_HonorsForwardedProtoFromTrustedPeer(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithTrustedProxies(mustPrefixes(t, "10.0.0.0/8")),
	).Handler()

	got := schemeFor(t, h, "10.1.2.3:443", "https")
	if got != "https" {
		t.Errorf("scheme = %q, want %q (forwarded by trusted proxy)", got, "https")
	}
}

func TestTrustedProxy_IgnoresForwardedProtoFromUntrustedPeer(t *testing.T) {
	h := httpapi.NewServer(
		httpapi.WithTrustedProxies(mustPrefixes(t, "10.0.0.0/8")),
	).Handler()

	// Peer is not trusted; the spoofed X-Forwarded-Proto must be ignored and
	// the plain-HTTP request scheme used instead.
	got := schemeFor(t, h, "203.0.113.42:54321", "https")
	if got != "http" {
		t.Errorf("scheme = %q, want %q (spoofed X-Forwarded-Proto ignored)", got, "http")
	}
}

func TestTrustedProxy_NoPrefixesIgnoresForwardedProto(t *testing.T) {
	h := httpapi.NewServer().Handler()

	got := schemeFor(t, h, "10.1.2.3:443", "https")
	if got != "http" {
		t.Errorf("scheme = %q, want %q (no trusted proxies configured)", got, "http")
	}
}
