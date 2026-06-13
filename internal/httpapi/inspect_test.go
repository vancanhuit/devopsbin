package httpapi_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/vancanhuit/devopsbin/internal/httpapi"
)

// uuidV4Pattern matches a canonical RFC 4122 version 4 UUID.
var uuidV4Pattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

// doGetWith issues a GET request, applying mutate to the request before it is
// served (e.g. to set headers or the remote address).
func doGetWith(t *testing.T, h http.Handler, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGetUuid(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/uuid")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := decode[httpapi.UuidResponse](t, rec)
	if !uuidV4Pattern.MatchString(body.Uuid.String()) {
		t.Errorf("uuid = %q, want a canonical UUIDv4", body.Uuid)
	}
	if body.Uuid.Version() != 4 {
		t.Errorf("uuid version = %d, want 4", body.Uuid.Version())
	}
}

func TestGetUuid_Unique(t *testing.T) {
	h := httpapi.NewServer().Handler()

	first := decode[httpapi.UuidResponse](t, doGet(t, h, "/api/v1/uuid"))
	second := decode[httpapi.UuidResponse](t, doGet(t, h, "/api/v1/uuid"))

	if first.Uuid == second.Uuid {
		t.Errorf("two calls returned the same uuid %q", first.Uuid)
	}
}

func TestGetIp(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGetWith(t, h, "/api/v1/ip", func(r *http.Request) {
		r.RemoteAddr = "203.0.113.42:54321"
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.IpResponse](t, rec)
	if body.Origin != "203.0.113.42" {
		t.Errorf("origin = %q, want %q", body.Origin, "203.0.113.42")
	}
}

func TestGetHeaders(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGetWith(t, h, "/api/v1/headers", func(r *http.Request) {
		r.Header.Set("X-Custom-Header", "demo-value")
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.HeadersResponse](t, rec)
	got, ok := body.Headers["X-Custom-Header"]
	if !ok {
		t.Fatalf("headers missing X-Custom-Header; got %v", body.Headers)
	}
	if len(got) != 1 || got[0] != "demo-value" {
		t.Errorf("X-Custom-Header = %v, want [demo-value]", got)
	}
}

func TestGetUserAgent(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGetWith(t, h, "/api/v1/user-agent", func(r *http.Request) {
		r.Header.Set("User-Agent", "devopsbin-test/1.0")
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.UserAgentResponse](t, rec)
	if body.UserAgent != "devopsbin-test/1.0" {
		t.Errorf("user-agent = %q, want %q", body.UserAgent, "devopsbin-test/1.0")
	}
}

func TestGetEcho(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGetWith(t, h, "/api/v1/echo?foo=bar&foo=baz&n=1", func(r *http.Request) {
		r.RemoteAddr = "203.0.113.42:54321"
		r.Header.Set("X-Custom-Header", "demo-value")
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.EchoResponse](t, rec)
	if body.Method != http.MethodGet {
		t.Errorf("method = %q, want %q", body.Method, http.MethodGet)
	}
	if body.Path != "/api/v1/echo" {
		t.Errorf("path = %q, want %q", body.Path, "/api/v1/echo")
	}
	if body.Origin != "203.0.113.42" {
		t.Errorf("origin = %q, want %q", body.Origin, "203.0.113.42")
	}
	if foo := body.Query["foo"]; len(foo) != 2 || foo[0] != "bar" || foo[1] != "baz" {
		t.Errorf("query[foo] = %v, want [bar baz]", foo)
	}
	if n := body.Query["n"]; len(n) != 1 || n[0] != "1" {
		t.Errorf("query[n] = %v, want [1]", n)
	}
	if got := body.Headers["X-Custom-Header"]; len(got) != 1 || got[0] != "demo-value" {
		t.Errorf("headers[X-Custom-Header] = %v, want [demo-value]", got)
	}
	if body.Scheme != httpapi.EchoResponseSchemeHttp {
		t.Errorf("scheme = %q, want %q (plain HTTP request)", body.Scheme, httpapi.EchoResponseSchemeHttp)
	}
	if body.Body != nil {
		t.Errorf("body = %v, want nil for a GET request", body.Body)
	}
}

// doBody issues a request with the given method, path, and body, applying
// mutate (if non-nil) before the request is served.
func doBody(
	t *testing.T,
	h http.Handler,
	method, path, reqBody string,
	mutate func(*http.Request),
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(reqBody))
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEchoReflectsBodyMethods(t *testing.T) {
	h := httpapi.NewServer().Handler()

	for _, method := range []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	} {
		t.Run(method, func(t *testing.T) {
			rec := doBody(t, h, method, "/api/v1/echo", "hello world", func(r *http.Request) {
				r.RemoteAddr = "203.0.113.42:54321"
			})

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			body := decode[httpapi.EchoResponse](t, rec)
			if body.Method != method {
				t.Errorf("method = %q, want %q", body.Method, method)
			}
			if body.Body == nil || *body.Body != "hello world" {
				t.Errorf("body = %v, want %q", body.Body, "hello world")
			}
			if body.Origin != "203.0.113.42" {
				t.Errorf("origin = %q, want %q", body.Origin, "203.0.113.42")
			}
		})
	}
}

func TestEchoEmptyBody(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doBody(t, h, http.MethodPost, "/api/v1/echo", "", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.EchoResponse](t, rec)
	if body.Body != nil {
		t.Errorf("body = %v, want nil for an empty request body", body.Body)
	}
}

func TestEchoBodyTooLarge(t *testing.T) {
	h := httpapi.NewServer().Handler()

	oversized := strings.Repeat("a", (64<<10)+1)
	rec := doBody(t, h, http.MethodPost, "/api/v1/echo", oversized, nil)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := decode[httpapi.ErrorResponse](t, rec)
	if body.Error == "" {
		t.Error("error message is empty, want a description")
	}
}

func TestGetScheme(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGet(t, h, "/api/v1/scheme")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := decode[httpapi.SchemeResponse](t, rec)
	if body.Scheme != httpapi.SchemeResponseSchemeHttp {
		t.Errorf("scheme = %q, want %q (plain HTTP request)", body.Scheme, httpapi.SchemeResponseSchemeHttp)
	}
}

func TestGetScheme_HTTPSWhenTLS(t *testing.T) {
	h := httpapi.NewServer().Handler()

	rec := doGetWith(t, h, "/api/v1/scheme", func(r *http.Request) {
		r.TLS = &tls.ConnectionState{} // simulate a TLS-terminated connection
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := decode[httpapi.SchemeResponse](t, rec)
	if body.Scheme != httpapi.SchemeResponseSchemeHttps {
		t.Errorf("scheme = %q, want %q (TLS connection)", body.Scheme, httpapi.SchemeResponseSchemeHttps)
	}
}
