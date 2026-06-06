package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// basePath is the API base path defined by the OpenAPI `servers` entry.
const basePath = "/api/v1"

// NewRouter builds a chi router with a standard middleware stack and mounts the
// generated OpenAPI endpoints under the API base path.
func NewRouter(si StrictServerInterface) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
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

// Handler returns an http.Handler routing the OpenAPI endpoints to this Server,
// wrapped with the standard middleware stack and mounted under the API base path.
func (s *Server) Handler() http.Handler {
	return NewRouter(s)
}
