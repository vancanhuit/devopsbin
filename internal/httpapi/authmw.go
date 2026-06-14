package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/vancanhuit/devopsbin/internal/auth"
)

// sessionProtectedOps lists the operations that require a valid session. For
// unsafe HTTP methods among these, a matching CSRF token is also required.
var sessionProtectedOps = map[string]bool{
	"PostAuthLogout": true,
	"GetAuthMe":      true,
}

// authMiddleware returns a strict-server middleware that enforces session and
// CSRF requirements per operation. It fails closed: any validation failure
// short-circuits with a 401 or 403 and the wrapped handler never runs.
func (s *Server) authMiddleware() StrictMiddlewareFunc {
	return func(next StrictHandlerFunc, operationID string) StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			if !sessionProtectedOps[operationID] {
				return next(ctx, w, r, request)
			}

			sess, ok := s.loadSession(ctx, r)
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return nil, nil
			}

			if isUnsafeMethod(r.Method) {
				provided := r.Header.Get(csrfHeaderName)
				if !auth.ValidCSRFToken(sess.CSRFToken, provided) {
					writeJSONError(w, http.StatusForbidden, "invalid or missing CSRF token")
					return nil, nil
				}
			}

			// Slide the idle window. A failure here means the session expired
			// or the store is unavailable; fail closed.
			if err := s.sessions.TouchSession(ctx, sess); err != nil {
				writeJSONError(w, http.StatusUnauthorized, "authentication required")
				return nil, nil
			}

			return next(withSession(ctx, sess), w, r, request)
		}
	}
}

// loadSession reads the session cookie from r and validates it. It returns the
// session and true on success, or false when no valid session is present.
func (s *Server) loadSession(ctx context.Context, r *http.Request) (auth.Session, bool) {
	if r == nil {
		return auth.Session{}, false
	}
	c, err := r.Cookie(s.authSettings.SessionCookieName)
	if err != nil || c.Value == "" {
		return auth.Session{}, false
	}
	sess, err := s.sessions.GetSession(ctx, c.Value)
	if err != nil {
		return auth.Session{}, false
	}
	return sess, true
}

// isUnsafeMethod reports whether method mutates state and therefore requires
// CSRF protection.
func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// writeJSONError writes an ErrorResponse body with the given status code.
func writeJSONError(w http.ResponseWriter, code int, message string) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(ErrorResponse{Error: message}); err != nil {
		http.Error(w, message, code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = buf.WriteTo(w)
}
