package httpapi

import (
	"context"
	"net/http"

	"github.com/vancanhuit/devopsbin/internal/auth"
)

// ctxKey is an unexported context key type to avoid collisions.
type ctxKey int

const (
	requestCtxKey ctxKey = iota
	clientIPCtxKey
	clientSchemeCtxKey
	sessionCtxKey
)

// withRequest stores the incoming *http.Request in its context so the
// strict-server handlers (which only receive a context.Context) can reflect
// request details such as headers, the origin IP, and the scheme.
func withRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), requestCtxKey, r)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestFrom returns the *http.Request stored by withRequest, or nil when no
// request is present in the context.
func requestFrom(ctx context.Context) *http.Request {
	r, _ := ctx.Value(requestCtxKey).(*http.Request)
	return r
}

// withSession returns a copy of ctx carrying the authenticated session, set by
// the auth middleware after a successful session validation.
func withSession(ctx context.Context, sess auth.Session) context.Context {
	return context.WithValue(ctx, sessionCtxKey, sess)
}

// sessionFrom returns the authenticated session stored by the auth middleware
// and whether one is present.
func sessionFrom(ctx context.Context) (auth.Session, bool) {
	sess, ok := ctx.Value(sessionCtxKey).(auth.Session)
	return sess, ok
}
