package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// errMiss is the sentinel returned by the fake store on a key miss.
var errMiss = errors.New("miss")

func isMiss(err error) bool { return errors.Is(err, errMiss) }

// fakeStore is an in-memory SessionStore that records the most recent TTL set
// per key so tests can assert the sliding/absolute TTL behavior.
type fakeStore struct {
	mu       sync.Mutex
	data     map[string]string
	ttls     map[string]time.Duration
	sets     map[string]map[string]struct{}
	counters map[string]int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		data:     map[string]string{},
		ttls:     map[string]time.Duration{},
		sets:     map[string]map[string]struct{}{},
		counters: map[string]int64{},
	}
}

func (f *fakeStore) Set(_ context.Context, key, value string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = value
	f.ttls[key] = ttl
	return nil
}

func (f *fakeStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return "", errMiss
	}
	return v, nil
}

func (f *fakeStore) Del(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
	delete(f.ttls, key)
	delete(f.sets, key)
	delete(f.counters, key)
	return nil
}

func (f *fakeStore) GetDel(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.data[key]
	if !ok {
		return "", errMiss
	}
	delete(f.data, key)
	delete(f.ttls, key)
	return v, nil
}

func (f *fakeStore) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counters[key]++
	n := f.counters[key]
	if n == 1 {
		f.ttls[key] = ttl
	}
	return n, nil
}

func (f *fakeStore) TTL(_ context.Context, key string) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ttls[key], nil
}

func (f *fakeStore) SAdd(_ context.Context, key, member string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sets[key] == nil {
		f.sets[key] = map[string]struct{}{}
	}
	f.sets[key][member] = struct{}{}
	f.ttls[key] = ttl
	return nil
}

func (f *fakeStore) SMembers(_ context.Context, key string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	members := make([]string, 0, len(f.sets[key]))
	for m := range f.sets[key] {
		members = append(members, m)
	}
	return members, nil
}

func (f *fakeStore) len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.data)
}

func (f *fakeStore) ttl(key string) (time.Duration, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.ttls[key]
	return d, ok
}

const (
	testIdleTTL     = 30 * time.Minute
	testAbsoluteTTL = 12 * time.Hour
)

func newTestManager(store SessionStore) *Manager {
	return NewManager(store, isMiss, testIdleTTL, testAbsoluteTTL)
}

var testIdentity = Identity{UserID: "u1", Username: "alice", Role: "user"}

func TestManager_CreateAndGetSession(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	sess, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ID == "" || sess.CSRFToken == "" {
		t.Fatalf("expected non-empty id and csrf token, got id=%q csrf=%q", sess.ID, sess.CSRFToken)
	}
	if sess.UserID != testIdentity.UserID || sess.Username != testIdentity.Username || sess.Role != testIdentity.Role {
		t.Fatalf("identity not stored: %+v", sess)
	}

	got, err := m.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != sess.ID || got.UserID != sess.UserID || got.CSRFToken != sess.CSRFToken {
		t.Fatalf("GetSession mismatch: got %+v want %+v", got, sess)
	}
}

func TestManager_CreateSession_RotatesID(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	a, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession a: %v", err)
	}
	b, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession b: %v", err)
	}
	if a.ID == b.ID {
		t.Fatal("expected distinct session ids")
	}
	if a.CSRFToken == b.CSRFToken {
		t.Fatal("expected distinct csrf tokens")
	}
}

func TestManager_GetSession_Miss(t *testing.T) {
	m := newTestManager(newFakeStore())
	_, err := m.GetSession(context.Background(), "nope")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestManager_GetSession_AbsoluteExpiry(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	created := time.Now()
	m.now = func() time.Time { return created }
	sess, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Advance past the absolute lifetime.
	m.now = func() time.Time { return created.Add(testAbsoluteTTL + time.Second) }
	if _, err := m.GetSession(ctx, sess.ID); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want ErrSessionExpired", err)
	}
	if store.len() != 0 {
		t.Fatal("expected expired session to be deleted from the store")
	}
}

func TestManager_TouchSession_SlidesIdleTTL(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	created := time.Now()
	m.now = func() time.Time { return created }
	sess, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Early in the lifetime the TTL is the full idle window.
	m.now = func() time.Time { return created.Add(time.Minute) }
	if err := m.TouchSession(ctx, sess); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	if ttl, _ := store.ttl(sessionKey(sess.ID)); ttl != testIdleTTL {
		t.Fatalf("ttl = %v, want %v", ttl, testIdleTTL)
	}
}

func TestManager_TouchSession_CapsAtAbsolute(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	created := time.Now()
	m.now = func() time.Time { return created }
	sess, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Near the absolute deadline the TTL is capped to the remaining time.
	remaining := 10 * time.Minute
	m.now = func() time.Time { return created.Add(testAbsoluteTTL - remaining) }
	if err := m.TouchSession(ctx, sess); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	if ttl, _ := store.ttl(sessionKey(sess.ID)); ttl != remaining {
		t.Fatalf("ttl = %v, want %v", ttl, remaining)
	}
}

func TestManager_TouchSession_ExpiredAbsolute(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	created := time.Now()
	m.now = func() time.Time { return created }
	sess, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	m.now = func() time.Time { return created.Add(testAbsoluteTTL + time.Second) }
	if err := m.TouchSession(ctx, sess); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want ErrSessionExpired", err)
	}
	if store.len() != 0 {
		t.Fatal("expected expired session to be deleted")
	}
}

func TestManager_DeleteSession(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	sess, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := m.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := m.GetSession(ctx, sess.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
}

func TestManager_RevokeUserSessions_ExceptOne(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	a, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession a: %v", err)
	}
	b, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession b: %v", err)
	}

	// Revoke every session for the user except b.
	if err := m.RevokeUserSessions(ctx, testIdentity.UserID, b.ID); err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}
	if _, err := m.GetSession(ctx, a.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("session a err = %v, want ErrSessionNotFound", err)
	}
	if _, err := m.GetSession(ctx, b.ID); err != nil {
		t.Fatalf("session b should survive, got %v", err)
	}
}

func TestManager_RevokeUserSessions_All(t *testing.T) {
	store := newFakeStore()
	m := newTestManager(store)
	ctx := context.Background()

	a, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession a: %v", err)
	}
	b, err := m.CreateSession(ctx, testIdentity)
	if err != nil {
		t.Fatalf("CreateSession b: %v", err)
	}

	// An empty exceptID revokes all sessions for the user.
	if err := m.RevokeUserSessions(ctx, testIdentity.UserID, ""); err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}
	if _, err := m.GetSession(ctx, a.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("session a err = %v, want ErrSessionNotFound", err)
	}
	if _, err := m.GetSession(ctx, b.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("session b err = %v, want ErrSessionNotFound", err)
	}
	if store.len() != 0 {
		t.Fatalf("expected all sessions deleted, got %d", store.len())
	}
}

func TestValidCSRFToken(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		provided string
		want     bool
	}{
		{"match", "abc", "abc", true},
		{"mismatch", "abc", "xyz", false},
		{"empty expected", "", "abc", false},
		{"empty provided", "abc", "", false},
		{"both empty", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidCSRFToken(tc.expected, tc.provided); got != tc.want {
				t.Fatalf("ValidCSRFToken(%q, %q) = %v, want %v", tc.expected, tc.provided, got, tc.want)
			}
		})
	}
}

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse", 4)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "correct horse" {
		t.Fatal("password was not hashed")
	}
	if !CheckPassword(hash, "correct horse") {
		t.Fatal("CheckPassword rejected the correct password")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("CheckPassword accepted a wrong password")
	}
}
