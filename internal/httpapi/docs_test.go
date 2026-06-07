package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	yaml "github.com/oasdiff/yaml3"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

const (
	swaggerIndex = "<!doctype html><html><body><div id=\"swagger-ui\"></div></body></html>"
	redocIndex   = "<!doctype html><html><body><redoc></redoc></body></html>"
	specYAML     = "openapi: 3.0.3\ninfo:\n  title: Test\n  version: 0.0.0\n"
)

// docsHandler builds a Server serving the OpenAPI document plus in-memory
// Swagger UI and Redoc bundles.
func docsHandler(t *testing.T) http.Handler {
	t.Helper()
	swaggerFS := fstest.MapFS{
		"index.html":           {Data: []byte(swaggerIndex)},
		"swagger-ui-bundle.js": {Data: []byte("// bundle")},
		"swagger-ui.css":       {Data: []byte("/* css */")},
		"initializer.js":       {Data: []byte("// init")},
	}
	redocFS := fstest.MapFS{
		"index.html":          {Data: []byte(redocIndex)},
		"redoc.standalone.js": {Data: []byte("// redoc")},
	}
	return httpapi.NewServer(
		httpapi.WithDocs([]byte(specYAML), swaggerFS, redocFS),
	).Handler()
}

func TestDocs_ServesOpenAPISpec(t *testing.T) {
	h := docsHandler(t)

	rec := doGet(t, h, "/api/v1/openapi.yaml")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if rec.Body.String() != specYAML {
		t.Errorf("body = %q, want the embedded spec", rec.Body.String())
	}
}

func TestDocs_SpecVersionSyncedWithBuild(t *testing.T) {
	swaggerFS := fstest.MapFS{"index.html": {Data: []byte(swaggerIndex)}}
	redocFS := fstest.MapFS{"index.html": {Data: []byte(redocIndex)}}
	h := httpapi.NewServer(
		httpapi.WithBuildInfo(httpapi.BuildInfo{Version: "v9.9.9-test"}),
		httpapi.WithDocs([]byte(specYAML), swaggerFS, redocFS),
	).Handler()

	rec := doGet(t, h, "/api/v1/openapi.yaml")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var doc struct {
		Info struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
	}
	if err := yaml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("parse served spec: %v", err)
	}
	if doc.Info.Version != "v9.9.9-test" {
		t.Errorf("info.version = %q, want the build version v9.9.9-test", doc.Info.Version)
	}
}

func TestDocs_SwaggerIndexServed(t *testing.T) {
	h := docsHandler(t)

	rec := doGet(t, h, "/swagger/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != swaggerIndex {
		t.Errorf("body = %q, want the swagger shell", rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("Content-Security-Policy header is missing")
	}
}

func TestDocs_RedocIndexServed(t *testing.T) {
	h := docsHandler(t)

	rec := doGet(t, h, "/redoc/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != redocIndex {
		t.Errorf("body = %q, want the redoc shell", rec.Body.String())
	}
}

func TestDocs_BarePrefixRedirectsToSlash(t *testing.T) {
	h := docsHandler(t)

	for _, prefix := range []string{"/swagger", "/redoc"} {
		rec := doGet(t, h, prefix)
		if rec.Code != http.StatusMovedPermanently {
			t.Errorf("%s status = %d, want %d", prefix, rec.Code, http.StatusMovedPermanently)
		}
		if loc := rec.Header().Get("Location"); loc != prefix+"/" {
			t.Errorf("%s Location = %q, want %q", prefix, loc, prefix+"/")
		}
	}
}

func TestDocs_RelaxedCSPOnlyOnUIRoutes(t *testing.T) {
	h := docsHandler(t)

	ui := doGet(t, h, "/swagger/")
	if csp := ui.Header().Get("Content-Security-Policy"); csp == "" ||
		!strings.Contains(csp, "worker-src") {
		t.Errorf("swagger CSP = %q, want a worker-src directive", csp)
	}

	// The spec route is not a UI asset route, so it keeps the global CSP
	// (no relaxed worker-src).
	spec := doGet(t, h, "/api/v1/openapi.yaml")
	if csp := spec.Header().Get("Content-Security-Policy"); strings.Contains(csp, "worker-src") {
		t.Errorf("spec CSP = %q, should not include worker-src", csp)
	}
}
