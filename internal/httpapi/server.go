package httpapi

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// Check inspects a single dependency and reports its current state. It is used
// by the readiness and startup probes to build their aggregated responses.
type Check func(ctx context.Context) DependencyCheck

// BuildInfo holds the build and version metadata reported by GetVersion.
// These values are typically injected at build time via -ldflags.
type BuildInfo struct {
	Service   string
	Version   string
	GitSHA    string
	BuildTime time.Time
	GoVersion string
}

// Server is a concrete implementation of StrictServerInterface backing the
// runtime, health probe, and build metadata endpoints.
type Server struct {
	build         BuildInfo
	logger        *slog.Logger
	readyChecks   map[string]Check
	startupChecks map[string]Check
}

// Option configures a Server.
type Option func(*Server)

// WithBuildInfo sets the build and version metadata reported by GetVersion.
func WithBuildInfo(b BuildInfo) Option {
	return func(s *Server) {
		s.build = b
	}
}

// WithLogger sets the structured logger used for request logging. When unset,
// the server falls back to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithReadinessCheck registers a dependency check run by the readiness probe.
func WithReadinessCheck(name string, c Check) Option {
	return func(s *Server) {
		s.readyChecks[name] = c
	}
}

// WithStartupCheck registers a dependency check run by the startup probe.
func WithStartupCheck(name string, c Check) Option {
	return func(s *Server) {
		s.startupChecks[name] = c
	}
}

// NewServer builds a Server with the provided options.
func NewServer(opts ...Option) *Server {
	s := &Server{
		readyChecks:   make(map[string]Check),
		startupChecks: make(map[string]Check),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	if s.build.GoVersion == "" {
		s.build.GoVersion = runtime.Version()
	}
	return s
}

// GetLivez implements the liveness probe. It is process-only and never inspects
// external dependencies.
func (s *Server) GetLivez(_ context.Context, _ GetLivezRequestObject) (GetLivezResponseObject, error) {
	return GetLivez200JSONResponse{Status: LivezResponseStatusOk}, nil
}

// GetReadyz implements the readiness probe. It runs all registered readiness
// checks and reports not_ready (503) if any dependency check reports an error.
func (s *Server) GetReadyz(ctx context.Context, _ GetReadyzRequestObject) (GetReadyzResponseObject, error) {
	checks, healthy := runChecks(ctx, s.readyChecks)
	if !healthy {
		return GetReadyz503JSONResponse{Status: NotReady, Checks: checks}, nil
	}
	return GetReadyz200JSONResponse{Status: Ready, Checks: checks}, nil
}

// GetStartupz implements the startup probe. It runs all registered startup
// checks and reports starting (503) until every dependency check succeeds.
func (s *Server) GetStartupz(ctx context.Context, _ GetStartupzRequestObject) (GetStartupzResponseObject, error) {
	checks, healthy := runChecks(ctx, s.startupChecks)
	if !healthy {
		return GetStartupz503JSONResponse{Status: Starting, Checks: checks}, nil
	}
	return GetStartupz200JSONResponse{Status: Started, Checks: checks}, nil
}

// GetVersion implements the build and version metadata endpoint.
func (s *Server) GetVersion(_ context.Context, _ GetVersionRequestObject) (GetVersionResponseObject, error) {
	return GetVersion200JSONResponse{
		Service:   s.build.Service,
		Version:   s.build.Version,
		GitSha:    s.build.GitSHA,
		BuildTime: s.build.BuildTime,
		GoVersion: s.build.GoVersion,
	}, nil
}

// runChecks executes every check and reports whether all of them are healthy.
// A check is considered unhealthy only when it reports an error status; a
// skipped status does not fail the aggregate.
func runChecks(ctx context.Context, checks map[string]Check) (map[string]DependencyCheck, bool) {
	results := make(map[string]DependencyCheck, len(checks))
	healthy := true
	for name, check := range checks {
		result := check(ctx)
		if result.Status == DependencyCheckStatusError {
			healthy = false
		}
		results[name] = result
	}
	return results, healthy
}

// Ensure Server satisfies the generated strict server interface.
var _ StrictServerInterface = (*Server)(nil)
