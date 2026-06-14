// Package auth implements cookie-session authentication: password hashing,
// opaque server-side sessions backed by a TTL store (Redis), and the
// session-bound CSRF token used for the double-submit defense.
//
// Sessions are server-authoritative: the client holds only an opaque session
// id (in an HttpOnly cookie) and a readable CSRF token; all session state lives
// in the store keyed by that id. This keeps the API stateless across replicas.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// sessionKeyPrefix namespaces session keys in the store so the format can
// evolve without colliding with other keyspaces.
const sessionKeyPrefix = "session:v1:"

// tokenBytes is the entropy (in bytes) of generated session ids and CSRF
// tokens before base64url encoding.
const tokenBytes = 32

// Sentinel errors returned by the session Manager.
var (
	// ErrSessionNotFound indicates no session exists for the given id (a store
	// miss), so the caller should treat the request as unauthenticated.
	ErrSessionNotFound = errors.New("auth: session not found")
	// ErrSessionExpired indicates the session exceeded its absolute lifetime
	// and was deleted; the caller should treat the request as unauthenticated.
	ErrSessionExpired = errors.New("auth: session expired")
)

// SessionStore is the subset of store operations the Manager needs. A cache
// client (Redis) satisfies it. Get must surface a miss in a way recognizable
// by the isMiss predicate passed to NewManager.
type SessionStore interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Del(ctx context.Context, key string) error
}

// Identity is the user information bound into a new session.
type Identity struct {
	UserID   string
	Username string
	Role     string
}

// Session is the server-side session record. The id is the store key and is
// never serialized into the stored value.
type Session struct {
	ID         string    `json:"-"`
	UserID     string    `json:"userId"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	CSRFToken  string    `json:"csrfToken"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

// Manager creates, validates, refreshes, and deletes sessions. It enforces a
// sliding idle timeout (the store TTL, refreshed on each use) and a hard
// absolute timeout (checked against CreatedAt).
type Manager struct {
	store       SessionStore
	isMiss      func(error) bool
	idleTTL     time.Duration
	absoluteTTL time.Duration
	now         func() time.Time
}

// NewManager builds a session Manager. isMiss reports whether an error from the
// store's Get represents a key miss (versus a real failure). idleTTL is the
// sliding inactivity window; absoluteTTL caps the total session lifetime.
func NewManager(store SessionStore, isMiss func(error) bool, idleTTL, absoluteTTL time.Duration) *Manager {
	return &Manager{
		store:       store,
		isMiss:      isMiss,
		idleTTL:     idleTTL,
		absoluteTTL: absoluteTTL,
		now:         time.Now,
	}
}

// CreateSession mints a new session (fresh opaque id and CSRF token) for the
// given identity and stores it with the idle TTL. Calling it on login or
// register rotates the session, since the returned id differs from any prior
// session.
func (m *Manager) CreateSession(ctx context.Context, id Identity) (Session, error) {
	sid, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return Session{}, err
	}

	now := m.now()
	sess := Session{
		ID:         sid,
		UserID:     id.UserID,
		Username:   id.Username,
		Role:       id.Role,
		CSRFToken:  csrf,
		CreatedAt:  now,
		LastSeenAt: now,
	}
	if err := m.save(ctx, sess, m.idleTTL); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// GetSession loads and validates the session for id. It returns
// ErrSessionNotFound on a store miss and ErrSessionExpired (deleting the
// session) when the absolute lifetime is exceeded.
func (m *Manager) GetSession(ctx context.Context, id string) (Session, error) {
	raw, err := m.store.Get(ctx, sessionKey(id))
	if err != nil {
		if m.isMiss(err) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("auth: get session: %w", err)
	}

	var sess Session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return Session{}, fmt.Errorf("auth: unmarshal session: %w", err)
	}
	sess.ID = id

	if m.now().Sub(sess.CreatedAt) > m.absoluteTTL {
		// Best-effort cleanup; the caller treats this as unauthenticated
		// regardless of the delete outcome.
		_ = m.DeleteSession(ctx, id)
		return Session{}, ErrSessionExpired
	}
	return sess, nil
}

// TouchSession refreshes the session's last-seen time and resets its store TTL,
// implementing the sliding idle window. The new TTL is capped so the key never
// outlives the absolute timeout. It returns ErrSessionExpired when no lifetime
// remains.
func (m *Manager) TouchSession(ctx context.Context, sess Session) error {
	now := m.now()
	ttl := m.remainingTTL(sess.CreatedAt, now)
	if ttl <= 0 {
		_ = m.DeleteSession(ctx, sess.ID)
		return ErrSessionExpired
	}
	sess.LastSeenAt = now
	return m.save(ctx, sess, ttl)
}

// DeleteSession removes the session for id. Deleting a missing session is not
// an error.
func (m *Manager) DeleteSession(ctx context.Context, id string) error {
	if err := m.store.Del(ctx, sessionKey(id)); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

// remainingTTL returns the store TTL to apply: the smaller of the idle window
// and the time left before the absolute timeout.
func (m *Manager) remainingTTL(createdAt, now time.Time) time.Duration {
	absRemaining := m.absoluteTTL - now.Sub(createdAt)
	if absRemaining < m.idleTTL {
		return absRemaining
	}
	return m.idleTTL
}

func (m *Manager) save(ctx context.Context, sess Session, ttl time.Duration) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("auth: marshal session: %w", err)
	}
	if err := m.store.Set(ctx, sessionKey(sess.ID), string(data), ttl); err != nil {
		return fmt.Errorf("auth: store session: %w", err)
	}
	return nil
}

// ValidCSRFToken reports whether provided matches expected using a
// constant-time comparison. Empty tokens never match.
func ValidCSRFToken(expected, provided string) bool {
	if expected == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

// HashPassword returns the bcrypt hash of password at the given cost.
func HashPassword(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// randomToken returns a base64url-encoded cryptographically random token.
func randomToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: read random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sessionKey(id string) string {
	return sessionKeyPrefix + id
}
