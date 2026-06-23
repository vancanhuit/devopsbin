package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/vancanhuit/devopsbin/internal/store"
)

// Transfer is an in-memory implementation of the store transfer used by the
// handler tests. It mirrors the real store's validation and error mapping
// (amount, identical accounts, existence, ownership, funds) without a database.
func (f *fakeUsers) Transfer(_ context.Context, p store.TransferParams) (store.TransferResult, error) {
	if p.AmountCents <= 0 || p.FromAccountID == p.ToAccountID {
		return store.TransferResult{}, store.ErrInvalidTransfer
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	from, ok := f.accounts[p.FromAccountID]
	if !ok {
		return store.TransferResult{}, store.ErrAccountNotFound
	}
	to, ok := f.accounts[p.ToAccountID]
	if !ok {
		return store.TransferResult{}, store.ErrAccountNotFound
	}
	if from.ownerUserID != p.SessionUserID {
		return store.TransferResult{}, store.ErrNotAccountOwner
	}
	if from.balanceCents < p.AmountCents {
		return store.TransferResult{}, store.ErrInsufficientFunds
	}

	from.balanceCents -= p.AmountCents
	to.balanceCents += p.AmountCents

	return store.TransferResult{
		TransferID:       testTransferID,
		FromAccountID:    p.FromAccountID,
		ToAccountID:      p.ToAccountID,
		FromBalanceCents: from.balanceCents,
		ToBalanceCents:   to.balanceCents,
		AmountCents:      p.AmountCents,
		CreatedAt:        time.Unix(0, 0).UTC(),
		Attempts:         1,
	}, nil
}

// seedAccount inserts an account owned by ownerUserID with the given balance so
// transfer tests can target it.
func (f *fakeUsers) seedAccount(id, ownerUserID, ownerUsername, name string, balanceCents int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts[id] = &fakeAccount{
		id:            id,
		ownerUserID:   ownerUserID,
		ownerUsername: ownerUsername,
		name:          name,
		balanceCents:  balanceCents,
	}
}

// balanceOf returns the seeded account's current balance for assertions.
func (f *fakeUsers) balanceOf(id string) (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.accounts[id]
	if !ok {
		return 0, false
	}
	return a.balanceCents, true
}

const (
	// testTransferID and the account ids below are fixed UUIDs used by the
	// transfer handler tests.
	testTransferID = "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4c00"
	fromAccountID  = "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4c01"
	toAccountID    = "018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4c02"
)

// transferBody is the JSON request body for POST /transfer.
type transferBody struct {
	FromAccountId string `json:"fromAccountId"`
	ToAccountId   string `json:"toAccountId"`
	AmountCents   int64  `json:"amountCents"`
}

// transferResult is the decoded JSON response from POST /transfer.
type transferResult struct {
	TransferId       string `json:"transferId"`
	FromAccountId    string `json:"fromAccountId"`
	ToAccountId      string `json:"toAccountId"`
	FromBalanceCents int64  `json:"fromBalanceCents"`
	ToBalanceCents   int64  `json:"toBalanceCents"`
	AmountCents      int64  `json:"amountCents"`
	Attempts         int    `json:"attempts"`
}

// loginUser seeds a regular user with the given id and logs in as them,
// returning the live CSRF token for subsequent mutating requests.
func (a *authTestServer) loginUser(t *testing.T, id, username, password string) string {
	t.Helper()
	a.users.seed(t, id, username, "user", password)
	a.doClose(t, http.MethodPost, "/api/v1/auth/login",
		credentials{Username: username, Password: password}, nil)
	return a.csrfToken(t)
}

func TestGetAccounts_RequiresSession(t *testing.T) {
	a := newAuthTestServer(t)

	resp := a.do(t, http.MethodGet, "/api/v1/accounts", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestGetAccounts_AsUser_OK(t *testing.T) {
	a := newAuthTestServer(t)
	a.loginUser(t, testUserID, "alice", "alicepass")
	a.users.seedAccount(fromAccountID, testUserID, "alice", "Checking", 100000)
	a.users.seedAccount(toAccountID, testAdminID, "bob", "Savings", 50000)

	resp := a.do(t, http.MethodGet, "/api/v1/accounts", nil, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Accounts []struct {
			Id           string `json:"id"`
			BalanceCents int64  `json:"balanceCents"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Accounts) != 2 {
		t.Fatalf("accounts len = %d, want 2", len(body.Accounts))
	}
}

func TestPostTransfer_HappyPath(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginUser(t, testUserID, "alice", "alicepass")
	a.users.seedAccount(fromAccountID, testUserID, "alice", "Checking", 100000)
	a.users.seedAccount(toAccountID, testAdminID, "bob", "Savings", 50000)

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: toAccountID, AmountCents: 2500}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var got transferResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.FromBalanceCents != 97500 || got.ToBalanceCents != 52500 {
		t.Fatalf("balances = (%d, %d), want (97500, 52500)", got.FromBalanceCents, got.ToBalanceCents)
	}
	if got.AmountCents != 2500 || got.Attempts != 1 {
		t.Fatalf("amount/attempts = (%d, %d), want (2500, 1)", got.AmountCents, got.Attempts)
	}
	if bal, _ := a.users.balanceOf(fromAccountID); bal != 97500 {
		t.Fatalf("source balance = %d, want 97500", bal)
	}
}

func TestPostTransfer_MissingCSRF_Forbidden(t *testing.T) {
	a := newAuthTestServer(t)
	a.loginUser(t, testUserID, "alice", "alicepass")
	a.users.seedAccount(fromAccountID, testUserID, "alice", "Checking", 100000)
	a.users.seedAccount(toAccountID, testAdminID, "bob", "Savings", 50000)

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: toAccountID, AmountCents: 2500}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestPostTransfer_NotOwner_Forbidden(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginUser(t, testUserID, "alice", "alicepass")
	// The source account is owned by someone else (testAdminID).
	a.users.seedAccount(fromAccountID, testAdminID, "bob", "Checking", 100000)
	a.users.seedAccount(toAccountID, testUserID, "alice", "Savings", 50000)

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: toAccountID, AmountCents: 2500}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestPostTransfer_InsufficientFunds_Conflict(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginUser(t, testUserID, "alice", "alicepass")
	a.users.seedAccount(fromAccountID, testUserID, "alice", "Checking", 1000)
	a.users.seedAccount(toAccountID, testAdminID, "bob", "Savings", 50000)

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: toAccountID, AmountCents: 2500}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestPostTransfer_AccountNotFound(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginUser(t, testUserID, "alice", "alicepass")
	a.users.seedAccount(fromAccountID, testUserID, "alice", "Checking", 100000)
	// Destination account is not seeded.

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: toAccountID, AmountCents: 2500}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestPostTransfer_SameAccount_BadRequest(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginUser(t, testUserID, "alice", "alicepass")
	a.users.seedAccount(fromAccountID, testUserID, "alice", "Checking", 100000)

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: fromAccountID, AmountCents: 2500}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPostTransfer_NonPositiveAmount_BadRequest(t *testing.T) {
	a := newAuthTestServer(t)
	csrf := a.loginUser(t, testUserID, "alice", "alicepass")
	a.users.seedAccount(fromAccountID, testUserID, "alice", "Checking", 100000)
	a.users.seedAccount(toAccountID, testAdminID, "bob", "Savings", 50000)

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: toAccountID, AmountCents: 0}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, map[string]string{csrfHeaderName: csrf})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPostTransfer_RequiresSession(t *testing.T) {
	a := newAuthTestServer(t)

	body := transferBody{FromAccountId: fromAccountID, ToAccountId: toAccountID, AmountCents: 2500}
	resp := a.do(t, http.MethodPost, "/api/v1/transfer", body, nil)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
