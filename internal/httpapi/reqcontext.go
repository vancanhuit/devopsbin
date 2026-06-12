package httpapi

import (
	"context"
	"net/http"
)

// ctxKey is an unexported context key type to avoid collisions.
type ctxKey int

const requestCtxKey ctxKey = iota

// withRequest stores the incoming *http.Request in its context so the
// strict-server handlers (which only receive a context.Context) can reflect
// request details such as headers and the origin IP.
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
