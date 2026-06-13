package config_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/config"
)

// envKeys are every environment variable Load reads. clearEnv unsets all of
// them so each test starts from a known, default state regardless of the
// ambient environment.
var envKeys = []string{
	"APP_VERSION",
	"APP_GIT_SHA",
	"APP_BUILD_TIME",
	"APP_LOG_LEVEL",
	"HTTP_ADDR",
	"HTTP_READ_TIMEOUT",
	"HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT",
	"HTTP_SHUTDOWN_TIMEOUT",
	"HTTP_REQUEST_TIMEOUT",
	"HTTP_TLS_CERT_FILE",
	"HTTP_TLS_KEY_FILE",
	"HTTP_TLS_CLIENT_CA_FILE",
	"HTTP_TRUSTED_PROXIES",
	"POSTGRES_URL",
	"REDIS_MODE",
	"REDIS_ADDRS",
	"REDIS_MASTER_NAME",
	"REDIS_DB",
	"REDIS_USERNAME",
	"REDIS_PASSWORD",
	"REDIS_TLS",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range envKeys {
		// t.Setenv records the original value for restoration after the
		// test; the immediate Unsetenv then clears it so envDefault values
		// apply during the test run.
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	if cfg.App.Version != "dev" {
		t.Errorf("App.Version = %q, want dev", cfg.App.Version)
	}
	if cfg.App.LogLevel != "info" {
		t.Errorf("App.LogLevel = %q, want info", cfg.App.LogLevel)
	}
	if cfg.Http.Addr != ":8080" {
		t.Errorf("Http.Addr = %q, want :8080", cfg.Http.Addr)
	}
	if cfg.Http.ReadTimeout != 5*time.Second {
		t.Errorf("Http.ReadTimeout = %v, want 5s", cfg.Http.ReadTimeout)
	}
	if cfg.Http.WriteTimeout != 10*time.Second {
		t.Errorf("Http.WriteTimeout = %v, want 10s", cfg.Http.WriteTimeout)
	}
	if cfg.Http.IdleTimeout != 60*time.Second {
		t.Errorf("Http.IdleTimeout = %v, want 60s", cfg.Http.IdleTimeout)
	}
	if cfg.Http.ShutdownTimeout != 15*time.Second {
		t.Errorf("Http.ShutdownTimeout = %v, want 15s", cfg.Http.ShutdownTimeout)
	}
	if cfg.Http.RequestTimeout != 60*time.Second {
		t.Errorf("Http.RequestTimeout = %v, want 60s", cfg.Http.RequestTimeout)
	}
	if cfg.Postgres.URL != "postgres://user:password@localhost:5432/dbname?sslmode=disable" {
		t.Errorf("Postgres.URL = %q, want default DSN", cfg.Postgres.URL)
	}
	if cfg.Redis.Mode != config.RedisStandalone {
		t.Errorf("Redis.Mode = %q, want %q", cfg.Redis.Mode, config.RedisStandalone)
	}
	if len(cfg.Redis.Addrs) != 1 || cfg.Redis.Addrs[0] != "localhost:6379" {
		t.Errorf("Redis.Addrs = %v, want [localhost:6379]", cfg.Redis.Addrs)
	}
	if cfg.Redis.DB != 0 {
		t.Errorf("Redis.DB = %d, want 0", cfg.Redis.DB)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_VERSION", "1.2.3")
	t.Setenv("APP_GIT_SHA", "abc123")
	t.Setenv("APP_BUILD_TIME", "2026-06-06T10:00:00Z")
	t.Setenv("APP_LOG_LEVEL", "debug")
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("HTTP_READ_TIMEOUT", "1s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "2s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "3s")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "4s")
	t.Setenv("HTTP_REQUEST_TIMEOUT", "30s")
	t.Setenv("POSTGRES_URL", "postgres://u:p@h:5432/db?sslmode=require")
	t.Setenv("REDIS_MODE", "cluster")
	t.Setenv("REDIS_ADDRS", "n1:6379,n2:6379,n3:6379")
	t.Setenv("REDIS_USERNAME", "appuser")
	t.Setenv("REDIS_PASSWORD", "s3cret")
	t.Setenv("REDIS_TLS", "true")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	want := config.Config{
		App: config.AppConfig{
			Version:   "1.2.3",
			GitSHA:    "abc123",
			BuildTime: "2026-06-06T10:00:00Z",
			LogLevel:  "debug",
		},
		Http: config.HttpConfig{
			Addr:            ":9000",
			ReadTimeout:     1 * time.Second,
			WriteTimeout:    2 * time.Second,
			IdleTimeout:     3 * time.Second,
			ShutdownTimeout: 4 * time.Second,
			RequestTimeout:  30 * time.Second,
		},
		Postgres: config.PostgresConfig{URL: "postgres://u:p@h:5432/db?sslmode=require"},
		Redis: config.RedisConfig{
			Mode:     config.RedisCluster,
			Addrs:    []string{"n1:6379", "n2:6379", "n3:6379"},
			Username: "appuser",
			Password: "s3cret",
			TLS:      true,
		},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("cfg = %+v\nwant %+v", cfg, want)
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_LOG_LEVEL", "verbose")

	if _, err := config.Load(); err == nil {
		t.Fatal("Load(): expected error for invalid log level, got nil")
	}
}

func TestValidate(t *testing.T) {
	base := func() config.Config {
		return config.Config{
			App:   config.AppConfig{LogLevel: "info"},
			Http:  config.HttpConfig{Addr: ":8080", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second, RequestTimeout: time.Second},
			Redis: config.RedisConfig{Mode: config.RedisStandalone, Addrs: []string{"localhost:6379"}},
		}
	}

	tests := []struct {
		name    string
		mutate  func(c *config.Config)
		wantErr bool
	}{
		{"valid", func(*config.Config) {}, false},
		{"empty addr", func(c *config.Config) { c.Http.Addr = "" }, true},
		{"zero read timeout", func(c *config.Config) { c.Http.ReadTimeout = 0 }, true},
		{"zero write timeout", func(c *config.Config) { c.Http.WriteTimeout = 0 }, true},
		{"zero idle timeout", func(c *config.Config) { c.Http.IdleTimeout = 0 }, true},
		{"zero shutdown timeout", func(c *config.Config) { c.Http.ShutdownTimeout = 0 }, true},
		{"zero request timeout", func(c *config.Config) { c.Http.RequestTimeout = 0 }, true},
		{"bad log level", func(c *config.Config) { c.App.LogLevel = "trace" }, true},
		{"invalid postgres url", func(c *config.Config) { c.Postgres.URL = "://bad" }, true},
		{"invalid redis mode", func(c *config.Config) { c.Redis.Mode = "proxy" }, true},
		{"empty redis addrs", func(c *config.Config) { c.Redis.Addrs = nil }, true},
		{"invalid redis addr", func(c *config.Config) { c.Redis.Addrs = []string{"no-port"} }, true},
		{"standalone multiple addrs", func(c *config.Config) {
			c.Redis.Addrs = []string{"a:6379", "b:6379"}
		}, true},
		{"sentinel without master name", func(c *config.Config) {
			c.Redis.Mode = config.RedisSentinel
			c.Redis.Addrs = []string{"s1:26379"}
		}, true},
		{"valid sentinel", func(c *config.Config) {
			c.Redis.Mode = config.RedisSentinel
			c.Redis.Addrs = []string{"s1:26379", "s2:26379"}
			c.Redis.MasterName = "mymaster"
		}, false},
		{"valid cluster", func(c *config.Config) {
			c.Redis.Mode = config.RedisCluster
			c.Redis.Addrs = []string{"n1:6379", "n2:6379", "n3:6379"}
		}, false},
		{"cluster with nonzero db", func(c *config.Config) {
			c.Redis.Mode = config.RedisCluster
			c.Redis.Addrs = []string{"n1:6379", "n2:6379"}
			c.Redis.DB = 1
		}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestValidate_TLS(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert.pem")
	key := filepath.Join(dir, "key.pem")
	for _, f := range []string{cert, key} {
		if err := os.WriteFile(f, []byte("pem"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	missing := filepath.Join(dir, "absent.pem")

	base := func() config.Config {
		return config.Config{
			App:   config.AppConfig{LogLevel: "info"},
			Http:  config.HttpConfig{Addr: ":8080", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second, RequestTimeout: time.Second},
			Redis: config.RedisConfig{Mode: config.RedisStandalone, Addrs: []string{"localhost:6379"}},
		}
	}

	tests := []struct {
		name    string
		cert    string
		key     string
		wantErr bool
	}{
		{"neither set", "", "", false},
		{"both set and readable", cert, key, false},
		{"cert without key", cert, "", true},
		{"key without cert", "", key, true},
		{"cert file missing", missing, key, true},
		{"key file missing", cert, missing, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			c.Http.TLSCertFile = tc.cert
			c.Http.TLSKeyFile = tc.key
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestHttpConfig_TLSEnabled(t *testing.T) {
	tests := []struct {
		name string
		cert string
		key  string
		want bool
	}{
		{"neither", "", "", false},
		{"both", "cert.pem", "key.pem", true},
		{"cert only", "cert.pem", "", false},
		{"key only", "", "key.pem", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := config.HttpConfig{TLSCertFile: tc.cert, TLSKeyFile: tc.key}
			if got := h.TLSEnabled(); got != tc.want {
				t.Errorf("TLSEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

// writeCertPEM writes a minimal self-signed certificate in PEM form to path so
// it parses as a real CA bundle (unlike the placeholder bytes used where only
// readability matters).
func writeCertPEM(t *testing.T, path string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestValidate_MTLS(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert.pem")
	key := filepath.Join(dir, "key.pem")
	for _, f := range []string{cert, key} {
		if err := os.WriteFile(f, []byte("pem"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	caCert := filepath.Join(dir, "ca.pem")
	writeCertPEM(t, caCert)
	badCA := filepath.Join(dir, "bad-ca.pem")
	if err := os.WriteFile(badCA, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write %s: %v", badCA, err)
	}
	missing := filepath.Join(dir, "absent.pem")

	base := func() config.Config {
		return config.Config{
			App:   config.AppConfig{LogLevel: "info"},
			Http:  config.HttpConfig{Addr: ":8080", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second, RequestTimeout: time.Second},
			Redis: config.RedisConfig{Mode: config.RedisStandalone, Addrs: []string{"localhost:6379"}},
		}
	}

	tests := []struct {
		name     string
		cert     string
		key      string
		clientCA string
		wantErr  bool
	}{
		{"no mtls", cert, key, "", false},
		{"valid client ca", cert, key, caCert, false},
		{"client ca without server tls", "", "", caCert, true},
		{"client ca file missing", cert, key, missing, true},
		{"client ca unparseable", cert, key, badCA, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			c.Http.TLSCertFile = tc.cert
			c.Http.TLSKeyFile = tc.key
			c.Http.TLSClientCAFile = tc.clientCA
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestHttpConfig_MTLSEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cert     string
		key      string
		clientCA string
		want     bool
	}{
		{"none", "", "", "", false},
		{"tls only", "cert.pem", "key.pem", "", false},
		{"client ca only", "", "", "ca.pem", false},
		{"tls and client ca", "cert.pem", "key.pem", "ca.pem", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := config.HttpConfig{TLSCertFile: tc.cert, TLSKeyFile: tc.key, TLSClientCAFile: tc.clientCA}
			if got := h.MTLSEnabled(); got != tc.want {
				t.Errorf("MTLSEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestValidate_TrustedProxies(t *testing.T) {
	base := func() config.Config {
		return config.Config{
			App:   config.AppConfig{LogLevel: "info"},
			Http:  config.HttpConfig{Addr: ":8080", ReadTimeout: time.Second, WriteTimeout: time.Second, IdleTimeout: time.Second, ShutdownTimeout: time.Second, RequestTimeout: time.Second},
			Redis: config.RedisConfig{Mode: config.RedisStandalone, Addrs: []string{"localhost:6379"}},
		}
	}

	tests := []struct {
		name    string
		proxies []string
		wantErr bool
	}{
		{"empty", nil, false},
		{"valid ipv4 cidr", []string{"10.0.0.0/8"}, false},
		{"valid ipv6 cidr", []string{"::1/128"}, false},
		{"multiple valid", []string{"10.0.0.0/8", "192.168.0.0/16"}, false},
		{"invalid cidr", []string{"not-a-cidr"}, true},
		{"bare ip without prefix", []string{"10.0.0.1"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			c.Http.TrustedProxies = tc.proxies
			err := c.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestHttpConfig_TrustedProxyPrefixes(t *testing.T) {
	h := config.HttpConfig{TrustedProxies: []string{"10.0.0.0/8", "", "192.168.0.0/16"}}
	got, err := h.TrustedProxyPrefixes()
	if err != nil {
		t.Fatalf("TrustedProxyPrefixes() = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(prefixes) = %d, want 2 (empty entries skipped)", len(got))
	}

	if _, err := (config.HttpConfig{TrustedProxies: []string{"bad"}}).TrustedProxyPrefixes(); err == nil {
		t.Errorf("TrustedProxyPrefixes() = nil error, want error for malformed CIDR")
	}
}

func TestRedacted(t *testing.T) {
	c := config.Config{
		Postgres: config.PostgresConfig{URL: "postgres://user:secret@host:5432/db"},
		Redis: config.RedisConfig{
			Mode:     config.RedisStandalone,
			Addrs:    []string{"host:6379"},
			Password: "topsecret",
		},
	}

	got := c.Redacted()

	if got.Postgres.URL != "postgres://user:REDACTED@host:5432/db" {
		t.Errorf("Postgres.URL = %q, want password redacted", got.Postgres.URL)
	}

	// The Redis password is json:"-" so it is never serialized; the struct
	// value is left intact by Redacted (callers must never log it directly).
	if got.Redis.Password != "topsecret" {
		t.Errorf("Redis.Password = %q, want unchanged", got.Redis.Password)
	}

	// The original must be untouched.
	if c.Postgres.URL != "postgres://user:secret@host:5432/db" {
		t.Errorf("Redacted mutated the receiver: %q", c.Postgres.URL)
	}
}

func TestRedacted_NoCredentials(t *testing.T) {
	c := config.Config{
		Postgres: config.PostgresConfig{URL: "postgres://host:5432/db"},
		Redis:    config.RedisConfig{Mode: config.RedisStandalone, Addrs: []string{"host:6379"}},
	}

	got := c.Redacted()

	if got.Postgres.URL != "postgres://host:5432/db" {
		t.Errorf("Postgres.URL = %q, want unchanged", got.Postgres.URL)
	}
	if len(got.Redis.Addrs) != 1 || got.Redis.Addrs[0] != "host:6379" {
		t.Errorf("Redis.Addrs = %v, want [host:6379]", got.Redis.Addrs)
	}
}
