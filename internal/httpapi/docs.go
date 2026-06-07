package httpapi

import (
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	yaml "github.com/oasdiff/yaml3"
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
// spec is the OpenAPI document bytes; version, when non-empty, overrides the
// document's info.version so the consoles report the running binary's version.
// swaggerFS and redocFS must each be rooted at the corresponding UI build
// output (containing an index.html).
func mountDocs(r chi.Router, spec []byte, version string, swaggerFS, redocFS fs.FS) {
	spec = specWithVersion(spec, version)

	r.Get(specPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", specContentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(spec)
	})

	mountDocsUI(r, swaggerPrefix, swaggerFS)
	mountDocsUI(r, redocPrefix, redocFS)
}

// specWithVersion returns spec with its info.version field set to version. It
// fails open: an empty version or any parse error returns the original bytes
// unchanged, since the documentation is non-critical and must never break the
// spec endpoint.
func specWithVersion(spec []byte, version string) []byte {
	if version == "" {
		return spec
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(spec, &doc); err != nil || len(doc.Content) == 0 {
		return spec
	}

	info := mappingValue(doc.Content[0], "info")
	if info == nil {
		return spec
	}
	ver := mappingValue(info, "version")
	if ver == nil {
		return spec
	}
	ver.SetString(version)

	patched, err := yaml.Marshal(&doc)
	if err != nil {
		return spec
	}
	return patched
}

// mappingValue returns the value node for key in a YAML mapping node, or nil
// when the node is not a mapping or the key is absent. Mapping content is a flat
// slice of alternating key/value nodes.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
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
