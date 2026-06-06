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

// NewRouter builds a chi router with a standard middleware stack and mounts the
// generated OpenAPI endpoints under the API base path. Request logging is
// emitted as structured JSON via the provided slog.Logger; a nil logger falls
// back to slog.Default().
func NewRouter(si StrictServerInterface, logger *slog.Logger) chi.Router {
	if logger == nil {
		logger = slog.Default()
	}

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(middleware.Timeout(60 * time.Second))

	handler := NewStrictHandler(si, nil)
	HandlerWithOptions(handler, ChiServerOptions{
		BaseURL:    basePath,
		BaseRouter: r,
	})

	return r
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
// served at `/` alongside the API.
func (s *Server) Handler() http.Handler {
	r := NewRouter(s, s.logger)
	if s.spaFS != nil {
		mountSPA(r, s.spaFS, s.spaIndex, s.logger)
	}
	return r
}
