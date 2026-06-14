package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/auth"
	"github.com/vancanhuit/devopsbin/internal/httpapi"
	"github.com/vancanhuit/devopsbin/internal/store"
)

const (
	sessionCookieName = "devopsbin_session"
	csrfCookieName    = "devopsbin_csrf"
	csrfHeaderName    = "X-CSRF-Token"
	testUserID        = "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed"
)

// errSessMiss is the sentinel returned by the fake session store on a miss.
var errSessMiss = errors.New("miss")

func sessMiss(err error) bool { return errors.Is(err, errSessMiss) }

// fakeSessionStore is an in-memory auth.SessionStore for handler tests.
type fakeSessionStore struct {
	mu   sync.Mutex
	data map[string]string
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{data: map[string]string{}}
}

func (f *fakeSessionStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
	return nil
}

func (f *fakeSessionStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return "", errSessMiss
	}
	return v, nil
}

func (f *fakeSessionStore) Del(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
	return nil
}

func (f *fakeSessionStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.data)
}

// fakeUsers is an in-memory userStore for handler tests.
type fakeUsers struct {
	mu     sync.Mutex
	byName map[string]store.UserWithHash
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byName: map[string]store.UserWithHash{}}
}

func (f *fakeUsers) RegisterUser(_ context.Context, p store.NewUser) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byName[p.Username]; ok {
		return store.User{}, store.ErrUsernameTaken
	}
	u := store.UserWithHash{
		User:         store.User{ID: testUserID, Username: p.Username, Role: p.Role},
		PasswordHash: p.PasswordHash,
	}
	f.byName[p.Username] = u
	return u.User, nil
}

func (f *fakeUsers) UserByUsername(_ context.Context, username string) (store.UserWithHash, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byName[username]
	if !ok {
		return store.UserWithHash{}, store.ErrUserNotFound
	}
	return u, nil
}

// authTestServer bundles the test HTTP server with its in-memory backends.
type authTestServer struct {
	srv      *httptest.Server
	client   *http.Client
	users    *fakeUsers
	sessions *fakeSessionStore
}

func newAuthTestServer(t *testing.T) *authTestServer {
	t.Helper()

	users := newFakeUsers()
	sessions := newFakeSessionStore()
	manager := auth.NewManager(sessions, sessMiss, 30*time.Minute, 12*time.Hour)

	handler := httpapi.NewServer(
		httpapi.WithAuth(users, manager, httpapi.AuthSettings{
			BcryptCost:         4,
			SessionCookieName:  sessionCookieName,
			CSRFCookieName:     csrfCookieName,
			SessionAbsoluteTTL: 12 * time.Hour,
		}),
	).Handler()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}

	return &authTestServer{
		srv:      srv,
		client:   &http.Client{Jar: jar},
		users:    users,
		sessions: sessions,
	}
}

func (a *authTestServer) do(t *testing.T, method, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(data)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, a.srv.URL+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// doClose runs a request and discards the response body. Used for setup calls
// (e.g. registering or logging in) where only the side effects matter.
func (a *authTestServer) doClose(t *testing.T, method, path string, body any, headers map[string]string) {
	t.Helper()
	resp := a.do(t, method, path, body, headers)
	_ = resp.Body.Close()
}

// csrfToken returns the value of the CSRF cookie currently held by the jar.
func (a *authTestServer) csrfToken(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(a.srv.URL + "/api/v1")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	for _, c := range a.client.Jar.Cookies(u) {
		if c.Name == csrfCookieName {
			return c.Value
		}
	}
	return ""
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userBody struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func decodeUser(t *testing.T, resp *http.Response) userBody {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	var u userBody
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	return u
}

func hasCookie(resp *http.Response, name string) bool {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return true
		}
	}
	return false
}

func clearsCookie(resp *http.Response, name string) bool {
	for _, c := range resp.Cookies() {
		if c.Name == name && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

func cookieByName(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestAuth_Register_Success(t *testing.T) {
	a := newAuthTestServer(t)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if !hasCookie(resp, sessionCookieName) || !hasCookie(resp, csrfCookieName) {
		t.Fatal("expected session and csrf cookies to be set")
	}
	u := decodeUser(t, resp)
	if u.Username != "alice" || u.Role != "user" || u.ID == "" {
		t.Fatalf("unexpected user body: %+v", u)
	}
	if a.sessions.count() != 1 {
		t.Fatalf("expected 1 session stored, got %d", a.sessions.count())
	}
}

// TestAuth_CSRFCookie_RootPath guards against scoping the readable CSRF cookie
// to /api/v1: the SPA is served from the site root, so a cookie pinned to the
// API prefix is invisible to document.cookie and the client can never echo the
// token, making every state-changing request fail CSRF.
func TestAuth_CSRFCookie_RootPath(t *testing.T) {
	a := newAuthTestServer(t)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)
	defer func() { _ = resp.Body.Close() }()

	csrf := cookieByName(resp, csrfCookieName)
	if csrf == nil {
		t.Fatal("expected a csrf cookie to be set")
	}
	if csrf.Path != "/" {
		t.Fatalf("csrf cookie path = %q, want %q", csrf.Path, "/")
	}
	if csrf.HttpOnly {
		t.Fatal("csrf cookie must be readable by JS (HttpOnly must be false)")
	}
}

func TestAuth_Register_DuplicateUsername(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "other123"}, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestAuth_Register_MissingFields(t *testing.T) {
	a := newAuthTestServer(t)
	resp := a.do(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "", Password: ""}, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestAuth_Login_Success(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "alicepass"}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !hasCookie(resp, sessionCookieName) || !hasCookie(resp, csrfCookieName) {
		t.Fatal("expected fresh session and csrf cookies")
	}
	_ = resp.Body.Close()
}

func TestAuth_Login_WrongPassword(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "nope"}, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_Login_UnknownUser(t *testing.T) {
	a := newAuthTestServer(t)
	resp := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "ghost", Password: "whatever1"}, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_Me_RequiresSession(t *testing.T) {
	a := newAuthTestServer(t)
	resp := a.do(t, http.MethodGet, "/api/v1/auth/me", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_Me_WithSession(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodGet, "/api/v1/auth/me", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	u := decodeUser(t, resp)
	if u.Username != "alice" || u.Role != "user" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestAuth_Logout_RequiresSession(t *testing.T) {
	a := newAuthTestServer(t)
	resp := a.do(t, http.MethodPost, "/api/v1/auth/logout", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_Logout_MissingCSRF(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/logout", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAuth_Logout_WrongCSRF(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{csrfHeaderName: "wrong-token"})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAuth_Logout_Success(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	token := a.csrfToken(t)
	if token == "" {
		t.Fatal("expected a csrf token in the jar")
	}

	resp := a.do(t, http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{csrfHeaderName: token})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if !clearsCookie(resp, sessionCookieName) || !clearsCookie(resp, csrfCookieName) {
		t.Fatal("expected logout to clear the auth cookies")
	}
	_ = resp.Body.Close()

	if a.sessions.count() != 0 {
		t.Fatalf("expected session deleted on logout, got %d", a.sessions.count())
	}

	// A subsequent /me must be unauthenticated.
	me := a.do(t, http.MethodGet, "/api/v1/auth/me", nil, nil)
	defer func() { _ = me.Body.Close() }()
	if me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-logout /me status = %d, want 401", me.StatusCode)
	}
}

func TestAuth_SafeMethod_SkipsCSRF(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	// GET /me is a safe method and must succeed without a CSRF header.
	resp := a.do(t, http.MethodGet, "/api/v1/auth/me", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestAuth_Login_RotatesSession(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)
	firstToken := a.csrfToken(t)

	a.doClose(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "alicepass"}, nil)
	secondToken := a.csrfToken(t)

	if firstToken == secondToken {
		t.Fatal("expected login to rotate the csrf token")
	}
	if a.sessions.count() != 1 {
		t.Fatalf("expected exactly 1 active session after rotation, got %d", a.sessions.count())
	}
}
