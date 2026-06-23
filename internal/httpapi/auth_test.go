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
	"strings"
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

// fakeSessionStore is an in-memory store used by the auth handler tests. It
// implements every store interface the auth package needs (sessions, the
// per-user session index, reset tokens, and lockout counters) so a single
// instance can back the session manager, recovery, and lockout.
type fakeSessionStore struct {
	mu      sync.Mutex
	data    map[string]string
	sets    map[string]map[string]struct{}
	ttls    map[string]time.Duration
	counter map[string]int64
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{
		data:    map[string]string{},
		sets:    map[string]map[string]struct{}{},
		ttls:    map[string]time.Duration{},
		counter: map[string]int64{},
	}
}

func (f *fakeSessionStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
	f.ttls[key] = ttl
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
	delete(f.sets, key)
	delete(f.ttls, key)
	delete(f.counter, key)
	return nil
}

func (f *fakeSessionStore) GetDel(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return "", errSessMiss
	}
	delete(f.data, key)
	delete(f.ttls, key)
	return v, nil
}

func (f *fakeSessionStore) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counter[key]++
	n := f.counter[key]
	if n == 1 {
		f.ttls[key] = ttl
	}
	return n, nil
}

func (f *fakeSessionStore) TTL(_ context.Context, key string) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ttls[key], nil
}

func (f *fakeSessionStore) SAdd(_ context.Context, key, member string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sets[key] == nil {
		f.sets[key] = map[string]struct{}{}
	}
	f.sets[key][member] = struct{}{}
	f.ttls[key] = ttl
	return nil
}

func (f *fakeSessionStore) SMembers(_ context.Context, key string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	members := make([]string, 0, len(f.sets[key]))
	for m := range f.sets[key] {
		members = append(members, m)
	}
	return members, nil
}

// count reports the number of stored sessions (keys under the session prefix),
// ignoring index sets, reset tokens, and lockout counters.
func (f *fakeSessionStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k := range f.data {
		if strings.HasPrefix(k, "session:v1:") {
			n++
		}
	}
	return n
}

// fakeUsers is an in-memory userStore for handler tests.
type fakeUsers struct {
	mu     sync.Mutex
	byName map[string]store.UserWithHash
	byID   map[string]string // user id -> username, for ID-keyed lookups

	// accounts holds seeded accounts keyed by account id, with the owner's user
	// id, so the transfer handler tests can exercise ownership and balance
	// checks without a database.
	accounts map[string]*fakeAccount
}

// fakeAccount is an in-memory account used by the transfer handler tests.
type fakeAccount struct {
	id            string
	ownerUserID   string
	ownerUsername string
	name          string
	balanceCents  int64
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{
		byName:   map[string]store.UserWithHash{},
		byID:     map[string]string{},
		accounts: map[string]*fakeAccount{},
	}
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
	f.byID[u.ID] = p.Username
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

func (f *fakeUsers) UserByID(_ context.Context, id string) (store.UserWithHash, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name, ok := f.byID[id]
	if !ok {
		return store.UserWithHash{}, store.ErrUserNotFound
	}
	return f.byName[name], nil
}

func (f *fakeUsers) UpdatePassword(_ context.Context, id, passwordHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	name, ok := f.byID[id]
	if !ok {
		return store.ErrUserNotFound
	}
	u := f.byName[name]
	u.PasswordHash = passwordHash
	f.byName[name] = u
	return nil
}

// authTestServer bundles the test HTTP server with its in-memory backends.
type authTestServer struct {
	srv      *httptest.Server
	client   *http.Client
	users    *fakeUsers
	sessions *fakeSessionStore
	lockout  *auth.Lockout
	recovery *auth.Recovery
}

func newAuthTestServer(t *testing.T) *authTestServer {
	t.Helper()

	users := newFakeUsers()
	sessions := newFakeSessionStore()
	manager := auth.NewManager(sessions, sessMiss, 30*time.Minute, 12*time.Hour)
	recovery := auth.NewRecovery(sessions, sessMiss, 15*time.Minute)
	lockout := auth.NewLockout(sessions, sessMiss, 5, 15*time.Minute, 15*time.Minute)

	handler := httpapi.NewServer(
		httpapi.WithAuth(users, manager, httpapi.AuthSettings{
			BcryptCost:         4,
			SessionCookieName:  sessionCookieName,
			CSRFCookieName:     csrfCookieName,
			SessionAbsoluteTTL: 12 * time.Hour,
		}),
		httpapi.WithPasswordRecovery(recovery),
		httpapi.WithLoginLockout(lockout),
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
		lockout:  lockout,
		recovery: recovery,
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

// resetResponse is the body returned by /auth/password/reset-request.
type resetResponse struct {
	Message string `json:"message"`
	Token   string `json:"token"`
}

func TestAuth_PasswordChange_Success(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)
	firstToken := a.csrfToken(t)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/password/change",
		map[string]string{"currentPassword": "alicepass", "newPassword": "alicepass2"},
		map[string]string{csrfHeaderName: firstToken})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !hasCookie(resp, sessionCookieName) || !hasCookie(resp, csrfCookieName) {
		t.Fatal("expected the session to be rotated with fresh cookies")
	}
	_ = resp.Body.Close()

	// The session was rotated, so the CSRF token changed.
	if a.csrfToken(t) == firstToken {
		t.Fatal("expected password change to rotate the csrf token")
	}
	if a.sessions.count() != 1 {
		t.Fatalf("expected exactly 1 active session after change, got %d", a.sessions.count())
	}

	// The old password no longer works; the new one does.
	a.doClose(t, http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{csrfHeaderName: a.csrfToken(t)})
	bad := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "alicepass"}, nil)
	if bad.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login with old password status = %d, want 401", bad.StatusCode)
	}
	_ = bad.Body.Close()
	good := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "alicepass2"}, nil)
	if good.StatusCode != http.StatusOK {
		t.Fatalf("login with new password status = %d, want 200", good.StatusCode)
	}
	_ = good.Body.Close()
}

func TestAuth_PasswordChange_WrongCurrent(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodPost, "/api/v1/auth/password/change",
		map[string]string{"currentPassword": "wrongpass", "newPassword": "alicepass2"},
		map[string]string{csrfHeaderName: a.csrfToken(t)})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAuth_PasswordChange_RequiresSession(t *testing.T) {
	a := newAuthTestServer(t)
	resp := a.do(t, http.MethodPost, "/api/v1/auth/password/change",
		map[string]string{"currentPassword": "alicepass", "newPassword": "alicepass2"}, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_PasswordReset_Roundtrip(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)
	// Log out so the reset flow stands on its own (no session needed).
	a.doClose(t, http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{csrfHeaderName: a.csrfToken(t)})

	req := a.do(t, http.MethodPost, "/api/v1/auth/password/reset-request",
		map[string]string{"username": "alice"}, nil)
	if req.StatusCode != http.StatusOK {
		t.Fatalf("reset-request status = %d, want 200", req.StatusCode)
	}
	var rr resetResponse
	if err := json.NewDecoder(req.Body).Decode(&rr); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	_ = req.Body.Close()
	if rr.Token == "" {
		t.Fatal("expected a reset token for an existing user")
	}

	reset := a.do(t, http.MethodPost, "/api/v1/auth/password/reset",
		map[string]string{"token": rr.Token, "newPassword": "alicepass3"}, nil)
	if reset.StatusCode != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", reset.StatusCode)
	}
	_ = reset.Body.Close()

	// The token is single-use: a second reset must report 410.
	again := a.do(t, http.MethodPost, "/api/v1/auth/password/reset",
		map[string]string{"token": rr.Token, "newPassword": "alicepass4"}, nil)
	if again.StatusCode != http.StatusGone {
		t.Fatalf("second reset status = %d, want 410", again.StatusCode)
	}
	_ = again.Body.Close()

	// The new password works.
	login := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "alicepass3"}, nil)
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login with reset password status = %d, want 200", login.StatusCode)
	}
	_ = login.Body.Close()
}

func TestAuth_PasswordResetRequest_UnknownUserNoToken(t *testing.T) {
	a := newAuthTestServer(t)
	resp := a.do(t, http.MethodPost, "/api/v1/auth/password/reset-request",
		map[string]string{"username": "ghost"}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var rr resetResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	_ = resp.Body.Close()
	if rr.Token != "" {
		t.Fatal("expected no token for an unknown user")
	}
}

func TestAuth_PasswordReset_InvalidToken(t *testing.T) {
	a := newAuthTestServer(t)
	resp := a.do(t, http.MethodPost, "/api/v1/auth/password/reset",
		map[string]string{"token": "nope", "newPassword": "whatever12"}, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want 410", resp.StatusCode)
	}
}

func TestAuth_Login_LocksOutAfterRepeatedFailures(t *testing.T) {
	a := newAuthTestServer(t)
	a.doClose(t, http.MethodPost, "/api/v1/auth/register", credentials{Username: "alice", Password: "alicepass"}, nil)
	a.doClose(t, http.MethodPost, "/api/v1/auth/logout", nil, map[string]string{csrfHeaderName: a.csrfToken(t)})

	// Five failed attempts (the configured threshold) trip the lock.
	for i := 0; i < 5; i++ {
		resp := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "wrong"}, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}

	// Now even the correct password is locked out with a Retry-After hint.
	locked := a.do(t, http.MethodPost, "/api/v1/auth/login", credentials{Username: "alice", Password: "alicepass"}, nil)
	defer func() { _ = locked.Body.Close() }()
	if locked.StatusCode != http.StatusLocked {
		t.Fatalf("status = %d, want 423", locked.StatusCode)
	}
	if locked.Header.Get("Retry-After") == "" {
		t.Fatal("expected a Retry-After header on a 423 response")
	}
}
