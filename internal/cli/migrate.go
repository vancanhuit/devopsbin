package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/vancanhuit/devopsbin/internal/config"
	"github.com/vancanhuit/devopsbin/internal/migrate"
)

// newMigrateCmd applies the embedded database migrations. Migrations are run
// only via this explicit command — the server never migrates on startup — so a
// deployment runs `devopsbin migrate up` as a discrete step. A Postgres
// session-level advisory lock serializes concurrent runners (multiple replicas
// or init containers), so it is safe to invoke from every instance.
func newMigrateCmd() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Apply database schema migrations",
		Commands: []*cli.Command{
			{
				Name:  "up",
				Usage: "Apply all pending migrations",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return withMigrator(func(m *migrate.Migrator) error {
						results, err := m.Up(ctx)
						if err != nil {
							return err
						}
						if len(results) == 0 {
							fmt.Println("database is up to date, no migrations applied")
							return nil
						}
						for _, r := range results {
							fmt.Printf("applied %s\n", r.String())
						}
						return nil
					})
				},
			},
			{
				Name:  "status",
				Usage: "Show the state of every migration",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return withMigrator(func(m *migrate.Migrator) error {
						status, err := m.Status(ctx)
						if err != nil {
							return err
						}
						tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
						// Writes to the tabwriter are buffered; any error is
						// surfaced by Flush below.
						_, _ = fmt.Fprintln(tw, "VERSION\tSTATE\tAPPLIED AT\tSOURCE")
						for _, s := range status {
							appliedAt := "pending"
							if !s.AppliedAt.IsZero() {
								appliedAt = s.AppliedAt.Format("2006-01-02 15:04:05")
							}
							_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n",
								s.Source.Version, s.State, appliedAt, s.Source.Path)
						}
						return tw.Flush()
					})
				},
			},
			{
				Name:  "version",
				Usage: "Print the current schema version",
				Action: func(ctx context.Context, _ *cli.Command) error {
					return withMigrator(func(m *migrate.Migrator) error {
						version, err := m.Version(ctx)
						if err != nil {
							return err
						}
						fmt.Println(version)
						return nil
					})
				},
			},
		},
	}
}

// withMigrator loads the configuration, opens a migration runner against the
// configured Postgres URL, invokes fn, and always closes the runner.
func withMigrator(fn func(m *migrate.Migrator) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m, err := migrate.New(cfg.Postgres.URL)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()

	return fn(m)
}
