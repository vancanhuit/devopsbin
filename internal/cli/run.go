package cli

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/httpapi"
	"github.com/vancanhuit/devopsbin/internal/logging"
)

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
			// shut down gracefully.
			ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			api := httpapi.NewServer(httpapi.WithBuildInfo(httpapi.BuildInfo{
				Service:   "devopsbin-api",
				Version:   version,
				GitSHA:    commit,
				BuildTime: parseBuildTime(buildDate),
			}))

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
				return err
			case <-ctx.Done():
				logger.Info("shutdown signal received, draining connections")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Http.ShutdownTimeout)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					return err
				}
				logger.Info("server stopped cleanly")
				return <-serveErr
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
