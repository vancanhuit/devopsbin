package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	openapispec "github.com/vancanhuit/devopsbin/api"
	"github.com/vancanhuit/devopsbin/internal/auth"
	"github.com/vancanhuit/devopsbin/internal/cache"
	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/httpapi"
	"github.com/vancanhuit/devopsbin/internal/logging"
	"github.com/vancanhuit/devopsbin/internal/ratelimit"
	"github.com/vancanhuit/devopsbin/internal/store"
	"github.com/vancanhuit/devopsbin/web"
)

// dependencyCheckTimeout bounds each dependency ping run by the readiness and
// startup probes so an unreachable dependency fails the check quickly instead
// of blocking the probe on a connection attempt.
const dependencyCheckTimeout = 2 * time.Second

func newRunCmd() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Run the backend HTTP API server",
		Action: func(ctx context.Context, _ *cli.Command) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			logger := logging.New(os.Stdout, cfg.App.LogLevel)
			logger.Info("starting devopsbin", "addr", cfg.Http.Addr)

			// Cancel the run context on SIGINT/SIGTERM so the server can
			// shut down gracefully. os.Interrupt is the portable alias for
			// syscall.SIGINT (and works on Windows); SIGTERM has no os
			// equivalent so we name it directly.
			ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()

			// Connect to the backing dependencies. Both pools connect lazily,
			// so a dependency that is temporarily down does not block startup;
			// the readiness probe reports their live status via Ping.
			db, err := store.New(ctx, cfg.Postgres.URL)
			if err != nil {
				return err
			}
			defer db.Close()

			rdb, err := cache.New(cfg.Redis)
			if err != nil {
				return err
			}
			defer func() { _ = rdb.Close() }()

			indexHTML, err := web.IndexHTML()
			if err != nil {
				return err
			}

			swaggerFS, err := web.SwaggerUIFS()
			if err != nil {
				return err
			}

			redocFS, err := web.RedocFS()
			if err != nil {
				return err
			}

			trustedProxies, err := cfg.Http.TrustedProxyPrefixes()
			if err != nil {
				return err
			}

			sessions := auth.NewManager(
				rdb,
				cache.IsMiss,
				cfg.Auth.SessionIdleTTL,
				cfg.Auth.SessionAbsoluteTTL,
			)
			recovery := auth.NewRecovery(rdb, cache.IsMiss, cfg.Auth.ResetTokenTTL)
			lockout := auth.NewLockout(
				rdb,
				cache.IsMiss,
				cfg.Auth.LoginMaxAttempts,
				cfg.Auth.LoginWindow,
				cfg.Auth.LockTTL,
			)
			limiter := ratelimit.New(
				rdb,
				"ratelimit",
				cfg.RateLimit.Limit,
				cfg.RateLimit.Window,
			)

			api := httpapi.NewServer(
				httpapi.WithLogger(logger),
				httpapi.WithSPA(web.DistFS(), indexHTML),
				httpapi.WithDocs(openapispec.Spec(), swaggerFS, redocFS),
				httpapi.WithBuildInfo(httpapi.BuildInfo{
					Service:   "devopsbin-api",
					Version:   version,
					GitSHA:    commit,
					BuildTime: parseBuildTime(buildTime),
					GoVersion: runtime.Version(),
				}),
				httpapi.WithReadinessCheck("postgres", httpapi.PingCheck(db, dependencyCheckTimeout)),
				httpapi.WithReadinessCheck("redis", httpapi.PingCheck(rdb, dependencyCheckTimeout)),
				httpapi.WithStartupCheck("postgres", httpapi.PingCheck(db, dependencyCheckTimeout)),
				httpapi.WithStartupCheck("redis", httpapi.PingCheck(rdb, dependencyCheckTimeout)),
				httpapi.WithRequestTimeout(cfg.Http.RequestTimeout),
				httpapi.WithTrustedProxies(trustedProxies),
				httpapi.WithAuth(db, sessions, httpapi.AuthSettings{
					BcryptCost:         cfg.Auth.BcryptCost,
					SessionCookieName:  cfg.Auth.SessionCookieName,
					CSRFCookieName:     cfg.Auth.CSRFCookieName,
					SessionAbsoluteTTL: cfg.Auth.SessionAbsoluteTTL,
				}),
				httpapi.WithPasswordRecovery(recovery),
				httpapi.WithLoginLockout(lockout),
				httpapi.WithRateLimiter(limiter),
			)

			srv := &http.Server{
				Addr:         cfg.Http.Addr,
				Handler:      api.Handler(),
				ReadTimeout:  cfg.Http.ReadTimeout,
				WriteTimeout: cfg.Http.WriteTimeout,
				IdleTimeout:  cfg.Http.IdleTimeout,
				// Bound the header read separately from the body so a slow
				// client cannot hold a connection open by trickling headers
				// (Slowloris). Reuse ReadTimeout as the upper bound.
				ReadHeaderTimeout: cfg.Http.ReadTimeout,
				// Cap request header size (default 1 MiB) to reject header
				// bombs before they consume memory.
				MaxHeaderBytes: 1 << 20,
				// Enforce a modern TLS floor when serving HTTPS directly. This
				// is ignored for plain-HTTP serving.
				TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			}

			// When mutual TLS is configured, require every client to present a
			// certificate signed by the configured CA bundle. This is ignored
			// for plain-HTTP serving (MTLSEnabled implies direct HTTPS).
			if cfg.Http.MTLSEnabled() {
				pool, err := cfg.Http.ClientCAPool()
				if err != nil {
					return err
				}
				srv.TLSConfig.ClientCAs = pool
				srv.TLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
			}

			scheme := "http"
			if cfg.Http.TLSEnabled() {
				scheme = "https"
			}
			logger.Info("serving", "scheme", scheme, "mtls", cfg.Http.MTLSEnabled(), "addr", cfg.Http.Addr)

			serveErr := make(chan error, 1)
			go func() {
				var err error
				if cfg.Http.TLSEnabled() {
					// Cert and key paths are loaded from disk; TLSConfig above
					// pins the minimum version.
					err = srv.ListenAndServeTLS(cfg.Http.TLSCertFile, cfg.Http.TLSKeyFile)
				} else {
					err = srv.ListenAndServe()
				}
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					serveErr <- err
					return
				}
				serveErr <- nil
			}()

			select {
			case err := <-serveErr:
				// The server stopped on its own (e.g. failed to bind the
				// listen address) before any shutdown signal arrived.
				return err
			case <-ctx.Done():
				logger.Info("shutdown signal received, draining connections",
					"timeout", cfg.Http.ShutdownTimeout)

				// Restore the default signal handling so a second
				// SIGINT/SIGTERM during draining terminates the process
				// immediately instead of being swallowed.
				stop()

				shutdownCtx, cancel := context.WithTimeout(
					context.Background(), cfg.Http.ShutdownTimeout)
				defer cancel()

				if err := srv.Shutdown(shutdownCtx); err != nil {
					// Graceful drain exceeded the deadline (or otherwise
					// failed). Force-close any remaining connections so the
					// process can exit instead of hanging.
					logger.Error("graceful shutdown failed, forcing close", "error", err)
					if closeErr := srv.Close(); closeErr != nil {
						return errors.Join(err, closeErr)
					}
					return err
				}

				// Surface any error the serving goroutine reported while we
				// were shutting down.
				if err := <-serveErr; err != nil {
					return err
				}
				logger.Info("server stopped cleanly")
				return nil
			}
		},
	}
}

// parseBuildTime parses the ldflags-injected build date, falling back to the
// zero time when it is absent or malformed.
func parseBuildTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
