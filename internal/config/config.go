// Package config loads and validates the service configuration.
//
// All settings come from environment variables (12-factor style), grouped by a
// per-section prefix: APP_, HTTP_, POSTGRES_, and REDIS_ (for example
// HTTP_ADDR or REDIS_MODE). Defaults are tuned for production; local
// development overrides individual variables via the environment.
package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// RedisMode selects the Redis client topology.
type RedisMode string

// Supported Redis topologies.
const (
	RedisStandalone RedisMode = "standalone"
	RedisCluster    RedisMode = "cluster"
	RedisSentinel   RedisMode = "sentinel"
)

type AppConfig struct {
	Version   string `env:"VERSION" envDefault:"dev" json:"version"`
	GitSHA    string `env:"GIT_SHA" envDefault:"none" json:"git_sha"`
	BuildTime string `env:"BUILD_TIME" envDefault:"none" json:"build_time"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info" json:"log_level"`
}

type HttpConfig struct {
	Addr            string        `env:"ADDR" envDefault:":8080" json:"addr"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"5s" json:"read_timeout"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"10s" json:"write_timeout"`
	IdleTimeout     time.Duration `env:"IDLE_TIMEOUT" envDefault:"60s" json:"idle_timeout"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s" json:"shutdown_timeout"`
	RequestTimeout  time.Duration `env:"REQUEST_TIMEOUT" envDefault:"60s" json:"request_timeout"`
	// TLSCertFile and TLSKeyFile point at a PEM certificate (chain) and its
	// private key. Set both to serve HTTPS directly; leave both empty to serve
	// plain HTTP (e.g. when TLS is terminated by a fronting proxy).
	TLSCertFile string `env:"TLS_CERT_FILE" json:"tls_cert_file"`
	TLSKeyFile  string `env:"TLS_KEY_FILE" json:"tls_key_file"`
	// TrustedProxies lists CIDR blocks of reverse proxies allowed to set
	// forwarding headers. Forwarded headers (X-Forwarded-For) are honored only
	// when the immediate peer falls within one of these ranges; an empty list
	// disables forwarded-header handling so the peer address is authoritative.
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:"," json:"trusted_proxies"`
}

// TLSEnabled reports whether direct HTTPS serving is configured.
func (h HttpConfig) TLSEnabled() bool {
	return h.TLSCertFile != "" && h.TLSKeyFile != ""
}

// TrustedProxyPrefixes parses the configured TrustedProxies CIDR blocks into
// netip prefixes. Empty entries are skipped; a malformed CIDR returns an error.
func (h HttpConfig) TrustedProxyPrefixes() ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(h.TrustedProxies))
	for _, c := range h.TrustedProxies {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		p, err := netip.ParsePrefix(c)
		if err != nil {
			return nil, fmt.Errorf("config: invalid trusted proxy CIDR %q: %w", c, err)
		}
		prefixes = append(prefixes, p)
	}
	return prefixes, nil
}

type PostgresConfig struct {
	URL string `env:"URL" envDefault:"postgres://user:password@localhost:5432/dbname?sslmode=disable" json:"url"`
}

type RedisConfig struct {
	// Mode selects the client topology: standalone, cluster, or sentinel.
	Mode RedisMode `env:"MODE" envDefault:"standalone" json:"mode"`
	// Addrs lists the node addresses as host:port. Standalone uses exactly one
	// entry; cluster uses them as slot-discovery seeds; sentinel uses them as
	// the sentinel addresses.
	Addrs []string `env:"ADDRS" envSeparator:"," envDefault:"localhost:6379" json:"addrs"`
	// MasterName is the monitored primary's name; required in sentinel mode.
	MasterName string `env:"MASTER_NAME" json:"master_name"`
	// DB is the logical database index. Standalone and sentinel only; a cluster
	// supports only DB 0.
	DB int `env:"DB" envDefault:"0" json:"db"`
	// Username authenticates the connection (Redis ACL); optional.
	Username string `env:"USERNAME" json:"username"`
	// Password authenticates the connection. Kept out of any URL and never
	// serialized so it cannot leak through logs or version output.
	Password string `env:"PASSWORD" json:"-"`
	// TLS enables an in-transit-encrypted connection.
	TLS bool `env:"TLS" envDefault:"false" json:"tls"`
}

// Config is the resolved service configuration.
type Config struct {
	App      AppConfig      `envPrefix:"APP_"`
	Http     HttpConfig     `envPrefix:"HTTP_"`
	Postgres PostgresConfig `envPrefix:"POSTGRES_"`
	Redis    RedisConfig    `envPrefix:"REDIS_"`
}

// Load reads the configuration from environment variables and applies the
// defaults. It returns an error if the resulting config fails validation.
func Load() (Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return Config{}, fmt.Errorf("config: parse env: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate returns a non-nil error when c contains an invalid setting.
func (c Config) Validate() error {
	switch strings.ToLower(c.App.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: invalid log_level %q", c.App.LogLevel)
	}

	if c.Http.Addr == "" {
		return fmt.Errorf("config: addr must not be empty")
	}

	if c.Postgres.URL != "" {
		if _, err := url.Parse(c.Postgres.URL); err != nil {
			return fmt.Errorf("config: postgres_url is not a valid URL: %w", err)
		}
	}

	if err := c.Redis.Validate(); err != nil {
		return err
	}

	if c.Http.ReadTimeout <= 0 {
		return fmt.Errorf("config: read_timeout must be positive")
	}
	if c.Http.WriteTimeout <= 0 {
		return fmt.Errorf("config: write_timeout must be positive")
	}
	if c.Http.IdleTimeout <= 0 {
		return fmt.Errorf("config: idle_timeout must be positive")
	}
	if c.Http.ShutdownTimeout <= 0 {
		return fmt.Errorf("config: shutdown_timeout must be positive")
	}
	if c.Http.RequestTimeout <= 0 {
		return fmt.Errorf("config: request_timeout must be positive")
	}

	if err := c.Http.validateTLS(); err != nil {
		return err
	}

	if _, err := c.Http.TrustedProxyPrefixes(); err != nil {
		return err
	}

	return nil
}

// validateTLS enforces that the cert and key are configured together and that
// both files are present and readable when set. Leaving both empty is valid and
// selects plain HTTP serving.
func (h HttpConfig) validateTLS() error {
	switch {
	case h.TLSCertFile == "" && h.TLSKeyFile == "":
		return nil
	case h.TLSCertFile == "":
		return fmt.Errorf("config: tls_key_file is set but tls_cert_file is empty")
	case h.TLSKeyFile == "":
		return fmt.Errorf("config: tls_cert_file is set but tls_key_file is empty")
	}

	for _, f := range [...]string{h.TLSCertFile, h.TLSKeyFile} {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("config: tls file %q is not readable: %w", f, err)
		}
	}
	return nil
}

// Validate checks the Redis settings for the selected topology.
func (r RedisConfig) Validate() error {
	switch r.Mode {
	case RedisStandalone, RedisCluster, RedisSentinel:
	default:
		return fmt.Errorf("config: invalid redis mode %q (want %q, %q or %q)",
			r.Mode, RedisStandalone, RedisCluster, RedisSentinel)
	}

	if len(r.Addrs) == 0 {
		return fmt.Errorf("config: redis addrs must not be empty")
	}
	for _, addr := range r.Addrs {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			return fmt.Errorf("config: invalid redis addr %q: %w", addr, err)
		}
	}

	switch r.Mode {
	case RedisStandalone:
		if len(r.Addrs) != 1 {
			return fmt.Errorf("config: standalone redis requires exactly one addr, got %d", len(r.Addrs))
		}
	case RedisSentinel:
		if r.MasterName == "" {
			return fmt.Errorf("config: redis master_name is required in sentinel mode")
		}
	case RedisCluster:
		if r.DB != 0 {
			return fmt.Errorf("config: redis db must be 0 in cluster mode, got %d", r.DB)
		}
	}

	return nil
}

// Redacted returns a copy of c with passwords stripped from URL-shaped
// fields, suitable for logging or printing. The Redis password is never
// serialized (json:"-"), so it needs no redaction here.
func (c Config) Redacted() Config {
	out := c
	out.Postgres.URL = redactURL(c.Postgres.URL)
	return out
}

func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		// If it's unparseable we'd rather not leak it: return a placeholder.
		return "<unparseable>"
	}
	if u.User != nil {
		if _, hasPwd := u.User.Password(); hasPwd {
			u.User = url.UserPassword(u.User.Username(), "REDACTED")
		}
	}
	return u.String()
}
