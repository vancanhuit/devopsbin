package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/netip"
	"runtime"
	"time"

	"github.com/vancanhuit/devopsbin/internal/auth"
	"github.com/vancanhuit/devopsbin/internal/store"
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

// userStore is the subset of persistence the auth handlers need. The concrete
// *store.Store satisfies it; tests can substitute a fake.
type userStore interface {
	RegisterUser(ctx context.Context, params store.NewUser) (store.User, error)
	UserByUsername(ctx context.Context, username string) (store.UserWithHash, error)
	UserByID(ctx context.Context, id string) (store.UserWithHash, error)
	UpdatePassword(ctx context.Context, id, passwordHash string) error
}

// AuthSettings configures the auth handlers and cookies.
type AuthSettings struct {
	// BcryptCost is the work factor used to hash new passwords.
	BcryptCost int
	// SessionCookieName names the HttpOnly session cookie.
	SessionCookieName string
	// CSRFCookieName names the readable CSRF cookie.
	CSRFCookieName string
	// SessionAbsoluteTTL caps the session lifetime and is used as the cookie
	// Max-Age so the browser discards the cookies no later than the server
	// expires the session.
	SessionAbsoluteTTL time.Duration
}

// Server is a concrete implementation of StrictServerInterface backing the
// runtime, health probe, and build metadata endpoints.
type Server struct {
	build          BuildInfo
	logger         *slog.Logger
	spaFS          fs.FS
	spaIndex       []byte
	docsSpec       []byte
	swaggerFS      fs.FS
	redocFS        fs.FS
	readyChecks    map[string]Check
	startupChecks  map[string]Check
	requestTimeout time.Duration
	trustedProxies []netip.Prefix
	users          userStore
	sessions       *auth.Manager
	authSettings   AuthSettings
	recovery       *auth.Recovery
	lockout        *auth.Lockout
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

// WithSPA serves an embedded single-page app: distFS must be rooted at the SPA
// build output (containing index.html and assets/), and indexHTML is the shell
// served at `/` and as the client-side routing fallback. When unset, no static
// assets are served and the router exposes only the API.
func WithSPA(distFS fs.FS, indexHTML []byte) Option {
	return func(s *Server) {
		s.spaFS = distFS
		s.spaIndex = indexHTML
	}
}

// WithDocs serves the API documentation: the raw OpenAPI document at
// /api/v1/openapi.yaml, the Swagger UI console at /swagger, and the Redoc
// console at /redoc. spec is the OpenAPI document bytes; swaggerFS and redocFS
// must each be rooted at the corresponding UI build output (containing an
// index.html). When unset, no documentation routes are registered.
func WithDocs(spec []byte, swaggerFS, redocFS fs.FS) Option {
	return func(s *Server) {
		s.docsSpec = spec
		s.swaggerFS = swaggerFS
		s.redocFS = redocFS
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

// WithRequestTimeout sets the per-request handling timeout enforced by the
// router middleware. A non-positive value falls back to the router default.
func WithRequestTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.requestTimeout = d
	}
}

// WithTrustedProxies configures the reverse-proxy CIDR prefixes whose
// forwarded headers are honored. When empty, forwarded headers are ignored and
// the connecting peer address is authoritative.
func WithTrustedProxies(prefixes []netip.Prefix) Option {
	return func(s *Server) {
		s.trustedProxies = prefixes
	}
}

// WithAuth wires the user store, session manager, and auth settings used by the
// authentication endpoints and the session/CSRF middleware. When unset, the
// auth endpoints are registered but their dependencies are nil; callers that
// serve the API must configure auth.
func WithAuth(users userStore, sessions *auth.Manager, settings AuthSettings) Option {
	return func(s *Server) {
		s.users = users
		s.sessions = sessions
		s.authSettings = settings
	}
}

// WithPasswordRecovery wires the password-reset token issuer used by the
// reset-request and reset endpoints. When unset, those endpoints report that
// recovery is unavailable.
func WithPasswordRecovery(recovery *auth.Recovery) Option {
	return func(s *Server) {
		s.recovery = recovery
	}
}

// WithLoginLockout wires the brute-force lockout used by the login endpoint.
// When unset, logins are not throttled.
func WithLoginLockout(lockout *auth.Lockout) Option {
	return func(s *Server) {
		s.lockout = lockout
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
