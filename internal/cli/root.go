// Package cli implements the urfave/cli-based command tree for the devopsbin
// binary. The root command does not run a server itself -- subcommands like
// `run` drive behavior.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

// Build metadata, injected at build time via -ldflags. Defaults are used for
// `go run` and local builds.
var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// Execute is the entry point used by main(). It builds the command tree and
// runs it, returning a process exit code.
func Execute() int {
	app := newApp()
	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func newApp() *cli.Command {
	return &cli.Command{
		Name:    "devopsbin",
		Usage:   "DevOpsBin backend service",
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, commit, buildTime),
		Commands: []*cli.Command{
			newRunCmd(),
			newHealthcheckCmd(),
		},
	}
}
