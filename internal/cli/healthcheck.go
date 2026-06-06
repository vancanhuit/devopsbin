package cli

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/urfave/cli/v3"
)

// newHealthcheckCmd is the binary's own liveness probe -- handy as a Docker
// HEALTHCHECK in distroless images that ship neither curl nor wget.
//
// Usage in the Dockerfile:
//
//	HEALTHCHECK CMD ["/devopsbin", "healthcheck"]
func newHealthcheckCmd() *cli.Command {
	return &cli.Command{
		Name:  "healthcheck",
		Usage: "Probe the local /livez endpoint and exit 0 when it returns 200",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "url",
				Value: "http://127.0.0.1:8080/api/v1/livez",
				Usage: "URL to probe",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Value: 2 * time.Second,
				Usage: "request timeout",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			url := cmd.String("url")
			timeout := cmd.Duration("timeout")

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return err
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("healthcheck: status %d", resp.StatusCode)
			}
			return nil
		},
	}
}
