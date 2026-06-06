// Command devopsbin is the entry point for the DevOpsBin backend service.
package main

import (
	"os"

	"github.com/vancanhuit/devopsbin/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
