package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// basePath is the API base path defined by the OpenAPI `servers` entry.
const basePath = "/api/v1"

// defaultRequestTimeout bounds request handling when NewRouter is called with a
// non-positive timeout.
const defaultRequestTimeout = 60 * time.Second

// NewRouter builds a chi router with a standard middleware stack and mounts the
// generated OpenAPI endpoints under the API base path. Request logging is
// emitted as structured JSON via the provided slog.Logger; a nil logger falls
// back to slog.Default(). requestTimeout bounds request handling; values <= 0
// fall back to defaultRequestTimeout.
func NewRouter(si StrictServerInterface, logger *slog.Logger, requestTimeout time.Duration) chi.Router {
	if logger == nil {
		logger = slog.Default()
	}
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(middleware.Timeout(requestTimeout))

	handler := NewStrictHandler(si, nil)
	HandlerWithOptions(handler, ChiServerOptions{
		BaseURL:    basePath,
		BaseRouter: r,
	})

	return r
}

// securityHeaders sets conservative security response headers on every
// response. The directives are safe for the embedded single-page console:
// assets are same-origin and there are no inline scripts.
func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; " +
		"style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
		"font-src 'self'; connect-src 'self'; object-src 'none'; " +
		"base-uri 'self'; frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// requestLogger returns a chi middleware that logs one structured JSON record
// per request via slog, replacing chi's default text logger.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			defer func() {
				logger.LogAttrs(r.Context(), slog.LevelInfo, "http request",
					slog.String("request_id", middleware.GetReqID(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("remote_addr", r.RemoteAddr),
					slog.Int("status", ww.Status()),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// Handler returns an http.Handler routing the OpenAPI endpoints to this Server,
// wrapped with the standard middleware stack and mounted under the API base
// path. When the server is configured with WithSPA, the embedded console is
// served at `/` alongside the API. When configured with WithDocs, the OpenAPI
// document and the Swagger UI / Redoc consoles are served too.
func (s *Server) Handler() http.Handler {
	r := NewRouter(s, s.logger, s.requestTimeout)
	if s.docsSpec != nil {
		mountDocs(r, s.docsSpec, s.swaggerFS, s.redocFS)
	}
	if s.spaFS != nil {
		mountSPA(r, s.spaFS, s.spaIndex, s.logger)
	}
	return r
}
