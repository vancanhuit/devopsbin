package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/vancanhuit/devopsbin/internal/store"
)

// testAdminID is a distinct UUID used to seed the admin user, so it never
// collides with the fixed id RegisterUser assigns to HTTP-registered users.
const testAdminID = "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bef"

// ListUsers returns every seeded user so the admin handler can render them.
func (f *fakeUsers) ListUsers(_ context.Context) ([]store.AdminUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.AdminUser, 0, len(f.byName))
	for _, u := range f.byName {
		out = append(out, store.AdminUser{ID: u.ID, Username: u.Username, Role: u.Role})
	}
	return out, nil
}

// ListAllAccounts returns the seeded accounts, ordered by owner username then
// account id to mirror the store query. ListTransfers is not exercised by these
// tests and returns an empty slice.
func (f *fakeUsers) ListAllAccounts(_ context.Context) ([]store.AdminAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.AdminAccount, 0, len(f.accounts))
	for _, a := range f.accounts {
		out = append(out, store.AdminAccount{
			ID:            a.id,
			OwnerUsername: a.ownerUsername,
			Name:          a.name,
			BalanceCents:  a.balanceCents,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OwnerUsername != out[j].OwnerUsername {
			return out[i].OwnerUsername < out[j].OwnerUsername
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (f *fakeUsers) ListTransfers(_ context.Context) ([]store.AdminTransfer, error) {
	return []store.AdminTransfer{}, nil
}

// seed inserts a user directly with the given id, role, and password (hashed at
// the test bcrypt cost) so a later /auth/login yields a session carrying that
// role — the public /auth/register endpoint always creates role "user".
func (f *fakeUsers) seed(t *testing.T, id, username, role, password string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byName[username] = store.UserWithHash{
		User:         store.User{ID: id, Username: username, Role: role},
		PasswordHash: string(hash),
	}
	f.byID[id] = username
}

// loginAdmin seeds an admin and logs in as that admin, returning the live CSRF
// token for subsequent mutating requests.
func (a *authTestServer) loginAdmin(t *testing.T) string {
	t.Helper()
	a.users.seed(t, testAdminID, "admin", "admin", "adminpass")
	a.doClose(t, http.MethodPost, "/api/v1/auth/login",
		credentials{Username: "admin", Password: "adminpass"}, nil)
	return a.csrfToken(t)
}

func TestAdmin_ListUsers_AsAdmin_OK(t *testing.T) {
	a := newAuthTestServer(t)
	a.loginAdmin(t)

	resp := a.do(t, http.MethodGet, "/api/v1/admin/users", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Users []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Users) != 1 || body.Users[0].Username != "admin" || body.Users[0].Role != "admin" {
		t.Fatalf("users = %+v, want a single admin", body.Users)
	}
}

func TestAdmin_ListUsers_AsUser_Forbidden(t *testing.T) {
	a := newAuthTestServer(t)
	// A plain registered user gets role "user".
	a.doClose(t, http.MethodPost, "/api/v1/auth/register",
		credentials{Username: "alice", Password: "alicepass"}, nil)

	resp := a.do(t, http.MethodGet, "/api/v1/admin/users", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdmin_ListUsers_Unauthenticated_Unauthorized(t *testing.T) {
	a := newAuthTestServer(t)

	resp := a.do(t, http.MethodGet, "/api/v1/admin/users", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdmin_ListAccountsAndTransfers_AsAdmin_OK(t *testing.T) {
	a := newAuthTestServer(t)
	a.loginAdmin(t)

	for _, path := range []string{"/api/v1/admin/accounts", "/api/v1/admin/transfers"} {
		resp := a.do(t, http.MethodGet, path, nil, nil)
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestAdmin_UnlockUser_ClearsLockout(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginAdmin(t)

	// Seed a victim (role user, fixed id) and lock its account. Seeding avoids
	// the cookie-jar clobber that /auth/register would cause by re-authenticating
	// as the victim. Recording failures with an empty IP locks only the
	// user-scoped key, so the user-scoped unlock fully clears it.
	a.users.seed(t, testUserID, "victim", "user", "victimpass")
	ctx := context.Background()
	for range 5 {
		a.lockout.RecordFailure(ctx, "victim", "")
	}
	if locked, _ := a.lockout.Locked(ctx, "victim", ""); !locked {
		t.Fatal("expected victim to be locked before unlock")
	}

	resp := a.do(t, http.MethodPost, "/api/v1/admin/users/"+testUserID+"/unlock", nil,
		map[string]string{csrfHeaderName: csrf})
	if resp.StatusCode != http.StatusNoContent {
		_ = resp.Body.Close()
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	_ = resp.Body.Close()

	if locked, _ := a.lockout.Locked(ctx, "victim", ""); locked {
		t.Fatal("expected victim to be unlocked after admin unlock")
	}
}

func TestAdmin_UnlockUser_UnknownID_NotFound(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginAdmin(t)

	// A syntactically valid UUID that no user owns.
	const ghostID = "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bff"
	resp := a.do(t, http.MethodPost, "/api/v1/admin/users/"+ghostID+"/unlock", nil,
		map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestAdmin_PasswordReset_MintsToken(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginAdmin(t)
	a.users.seed(t, testUserID, "victim", "user", "victimpass")

	resp := a.do(t, http.MethodPost, "/api/v1/admin/users/"+testUserID+"/password-reset", nil,
		map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Token == "" {
		t.Fatal("expected a non-empty reset token in the response")
	}
}

func TestAdmin_RequiresCSRF_OnUnlock(t *testing.T) {
	a := newAuthTestServer(t)
	a.loginAdmin(t)

	// No CSRF header on an unsafe admin request must be rejected before RBAC.
	resp := a.do(t, http.MethodPost, "/api/v1/admin/users/"+testUserID+"/unlock", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (missing CSRF)", resp.StatusCode)
	}
}
