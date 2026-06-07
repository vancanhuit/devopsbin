package httpapi

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// API documentation routes.
const (
	// specPath serves the raw OpenAPI document. It lives under the API base
	// path so it sits alongside the operations it describes.
	specPath = basePath + "/openapi.yaml"
	// swaggerPrefix and redocPrefix mount the two embedded documentation UIs.
	swaggerPrefix = "/swagger"
	redocPrefix   = "/redoc"
)

// specContentType is the media type for the served OpenAPI document.
const specContentType = "application/yaml"

// docsCSP relaxes the global Content-Security-Policy for the embedded
// documentation UIs only. Both consoles are served entirely from same-origin
// vendored bundles (no CDN), but Redoc renders the spec in a web worker created
// from a blob: URL, so worker-src blob: is required; both inject inline styles
// and use data: fonts/images. Everything else stays as locked-down as the
// global policy.
const docsCSP = "default-src 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
	"font-src 'self' data:; worker-src 'self' blob:; connect-src 'self'; " +
	"object-src 'none'; base-uri 'self'; frame-ancestors 'none'"

// mountDocs registers the API documentation routes on r:
//
//   - GET /api/v1/openapi.yaml — the raw OpenAPI document.
//   - GET /swagger[/...]       — the embedded Swagger UI console.
//   - GET /redoc[/...]         — the embedded Redoc console.
//
// spec is the OpenAPI document bytes; swaggerFS and redocFS must each be rooted
// at the corresponding UI build output (containing an index.html).
func mountDocs(r chi.Router, spec []byte, swaggerFS, redocFS fs.FS) {
	r.Get(specPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", specContentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(spec)
	})

	mountDocsUI(r, swaggerPrefix, swaggerFS)
	mountDocsUI(r, redocPrefix, redocFS)
}

// mountDocsUI serves a static documentation UI rooted at uiFS under prefix. The
// bare prefix redirects to prefix+"/" so the UI's relative asset URLs resolve.
func mountDocsUI(r chi.Router, prefix string, uiFS fs.FS) {
	fileServer := http.StripPrefix(prefix, docsHeaders(http.FileServer(http.FS(uiFS))))

	r.Get(prefix, func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, prefix+"/", http.StatusMovedPermanently)
	})
	r.Get(prefix+"/*", fileServer.ServeHTTP)
}

// docsHeaders applies the documentation-specific CSP to every response, relaxing
// the global policy just enough for the embedded UIs to run.
func docsHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", docsCSP)
		next.ServeHTTP(w, r)
	})
}
