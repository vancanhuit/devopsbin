package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
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
			&cli.StringFlag{
				Name: "cacert",
				Usage: "path to a PEM CA bundle used to verify the server " +
					"certificate when probing an https URL",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			url := cmd.String("url")
			timeout := cmd.Duration("timeout")

			client, err := healthcheckClient(cmd.String("cacert"), timeout)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return err
			}

			resp, err := client.Do(req)
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

// healthcheckClient builds the HTTP client used by the probe. When caCertFile
// is set, the server certificate is verified against that PEM bundle (mirroring
// production behaviour with a private CA) instead of the system trust store.
// An empty caCertFile yields a default client suitable for plain HTTP.
func healthcheckClient(caCertFile string, timeout time.Duration) (*http.Client, error) {
	if caCertFile == "" {
		return &http.Client{Timeout: timeout}, nil
	}

	pem, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("healthcheck: read cacert: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("healthcheck: no certificates found in %q", caCertFile)
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    pool,
			},
		},
	}, nil
}
