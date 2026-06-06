package httpapi_test

import (
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

// spaHandler builds a Server serving an in-memory SPA bundle: an index.html
// shell plus a single hashed asset under assets/.
func spaHandler(t *testing.T) http.Handler {
	t.Helper()
	const indexHTML = "<!doctype html><html><body><div id=\"app\"></div></body></html>"
	dist := fstest.MapFS{
		"index.html":             {Data: []byte(indexHTML)},
		"assets/index-abc123.js": {Data: []byte("console.log('hi')")},
	}
	return httpapi.NewServer(httpapi.WithSPA(dist, []byte(indexHTML))).Handler()
}

func TestSPA_IndexServesShell(t *testing.T) {
	h := spaHandler(t)

	rec := doGet(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if rec.Header().Get("Last-Modified") == "" {
		t.Error("Last-Modified header is missing")
	}
}

func TestSPA_AssetIsImmutable(t *testing.T) {
	h := spaHandler(t)

	rec := doGet(t, h, "/assets/index-abc123.js")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want immutable directive", cc)
	}
}

func TestSPA_MissingAssetIs404(t *testing.T) {
	h := spaHandler(t)

	rec := doGet(t, h, "/assets/does-not-exist.js")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSPA_DeepLinkFallsBackToShell(t *testing.T) {
	h := spaHandler(t)

	rec := doGet(t, h, "/some/client/route")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want the SPA shell", ct)
	}
}

func TestSPA_UnknownAPIRouteStays404(t *testing.T) {
	h := spaHandler(t)

	// API paths must keep real 404 semantics rather than being masked by the
	// SPA shell fallback.
	rec := doGet(t, h, "/api/v1/does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestSPA_SecurityHeadersOnShell(t *testing.T) {
	h := spaHandler(t)

	rec := doGet(t, h, "/")

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy header is missing on the SPA shell")
	}
}
