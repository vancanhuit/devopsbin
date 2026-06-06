// Package config loads and validates the service configuration.
//
// All settings come from environment variables prefixed with DEVOPSBIN_
// (12-factor style). Defaults are tuned for production; local development
// overrides them via the environment.
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Environment values.
const (
	EnvDev  = "dev"
	EnvProd = "prod"
)

type AppConfig struct {
	Environment string `env:"ENV" envDefault:"prod" json:"environment"`
	Version     string `env:"VERSION" envDefault:"dev" json:"version"`
	GitSHA      string `env:"GIT_SHA" envDefault:"none" json:"git_sha"`
	BuildTime   string `env:"BUILD_TIME" envDefault:"none" json:"build_time"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info" json:"log_level"`
}

type HttpConfig struct {
	Addr            string        `env:"ADDR" envDefault:":8080" json:"addr"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"5s" json:"read_timeout"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"10s" json:"write_timeout"`
	IdleTimeout     time.Duration `env:"IDLE_TIMEOUT" envDefault:"60s" json:"idle_timeout"`
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s" json:"shutdown_timeout"`
}

type PostgresConfig struct {
	URL string `env:"URL" envDefault:"postgres://user:password@localhost:5432/dbname?sslmode=disable" json:"url"`
}

type RedisConfig struct {
	URL string `env:"URL" envDefault:"redis://localhost:6379/0" json:"url"`
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
	switch c.App.Environment {
	case EnvDev, EnvProd:
	default:
		return fmt.Errorf("config: invalid env %q (want %q or %q)", c.App.Environment, EnvDev, EnvProd)
	}

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
	if c.Redis.URL != "" {
		if _, err := url.Parse(c.Redis.URL); err != nil {
			return fmt.Errorf("config: redis_url is not a valid URL: %w", err)
		}
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

	return nil
}

// Redacted returns a copy of c with passwords stripped from URL-shaped
// fields, suitable for logging or printing.
func (c Config) Redacted() Config {
	out := c
	out.Postgres.URL = redactURL(c.Postgres.URL)
	out.Redis.URL = redactURL(c.Redis.URL)
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
