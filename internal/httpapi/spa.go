package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Cache-Control values for the embedded SPA assets.
const (
	// indexCacheControl keeps the SPA shell uncached so a redeploy is picked
	// up on the next navigation.
	indexCacheControl = "no-cache"
	// hashedAssetCacheControl marks the Vite content-hashed bundles as
	// immutable: their filenames change whenever their contents do.
	hashedAssetCacheControl = "public, max-age=31536000, immutable"
)

// spa serves the embedded Svelte console: the SPA shell at `/`, content-hashed
// bundles under `/assets/`, and the shell as a fallback for any other
// non-API GET route.
type spa struct {
	dist         fs.FS
	indexHTML    []byte
	lastModified time.Time
	logger       *slog.Logger
}

// mountSPA registers the SPA + asset routes on r. distFS must be rooted at the
// SPA build output (containing index.html and assets/).
func mountSPA(r chi.Router, distFS fs.FS, indexHTML []byte, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	s := &spa{
		dist:         distFS,
		indexHTML:    indexHTML,
		lastModified: time.Now().UTC(),
		logger:       logger,
	}

	assetsFS, err := fs.Sub(distFS, "assets")
	if err != nil {
		panic("httpapi: SPA embed missing assets dir: " + err.Error())
	}
	assetsHandler := http.StripPrefix("/assets/",
		cacheHeaders(http.FileServer(http.FS(assetsFS)), hashedAssetCacheControl))

	r.Get("/assets/*", assetsHandler.ServeHTTP)
	r.Get("/", s.index)
	// Serve the SPA shell for any unmatched route so deep links and client
	// navigation resolve, while leaving API and asset 404s intact.
	r.NotFound(s.fallback)
}

// index serves the SPA shell.
func (s *spa) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", indexCacheControl)
	w.Header().Set("Last-Modified", s.lastModified.Format(http.TimeFormat))
	w.Header().Set("Content-Length", strconv.Itoa(len(s.indexHTML)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.indexHTML)
}

// fallback serves the SPA shell for unmatched GET routes, but preserves 404
// semantics for API and asset paths so missing resources are not masked by the
// HTML shell.
func (s *spa) fallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet ||
		strings.HasPrefix(r.URL.Path, basePath) ||
		strings.HasPrefix(r.URL.Path, "/assets/") {
		http.NotFound(w, r)
		return
	}
	s.index(w, r)
}

// cacheHeaders sets a Cache-Control header on every response from next.
func cacheHeaders(next http.Handler, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(w, r)
	})
}
