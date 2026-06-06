package cli

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/vancanhuit/devopsbin/internal/cache"
	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/httpapi"
	"github.com/vancanhuit/devopsbin/internal/logging"
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
			logger.Info("starting devopsbin", "env", cfg.App.Environment, "addr", cfg.Http.Addr)

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

			rdb, err := cache.New(cfg.Redis.URL)
			if err != nil {
				return err
			}
			defer func() { _ = rdb.Close() }()

			indexHTML, err := web.IndexHTML()
			if err != nil {
				return err
			}

			api := httpapi.NewServer(
				httpapi.WithLogger(logger),
				httpapi.WithSPA(web.DistFS(), indexHTML),
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
			)

			srv := &http.Server{
				Addr:         cfg.Http.Addr,
				Handler:      api.Handler(),
				ReadTimeout:  cfg.Http.ReadTimeout,
				WriteTimeout: cfg.Http.WriteTimeout,
				IdleTimeout:  cfg.Http.IdleTimeout,
			}

			serveErr := make(chan error, 1)
			go func() {
				if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
