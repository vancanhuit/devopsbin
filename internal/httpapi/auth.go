package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/vancanhuit/devopsbin/internal/auth"
	"github.com/vancanhuit/devopsbin/internal/store"
)

// csrfHeaderName is the request header carrying the CSRF token for the
// double-submit defense.
const csrfHeaderName = "X-CSRF-Token"

// defaultUserRole is assigned to users created through self-registration.
const defaultUserRole = "user"

// PostAuthRegister creates a user and opens an authenticated session, setting
// the session and CSRF cookies.
func (s *Server) PostAuthRegister(ctx context.Context, request PostAuthRegisterRequestObject) (PostAuthRegisterResponseObject, error) {
	if request.Body == nil {
		return PostAuthRegister400JSONResponse{Error: "request body is required"}, nil
	}
	username := strings.TrimSpace(request.Body.Username)
	password := request.Body.Password
	if username == "" || password == "" {
		return PostAuthRegister400JSONResponse{Error: "username and password are required"}, nil
	}

	hash, err := auth.HashPassword(password, s.authSettings.BcryptCost)
	if err != nil {
		return nil, err
	}

	user, err := s.users.RegisterUser(ctx, store.NewUser{
		Username:     username,
		PasswordHash: hash,
		Role:         defaultUserRole,
	})
	if err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			return PostAuthRegister409JSONResponse{Error: "username is already taken"}, nil
		}
		return nil, err
	}

	resp, err := s.startSession(ctx, user, http.StatusCreated)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// PostAuthLogin verifies credentials and opens an authenticated session,
// rotating any existing session and setting fresh cookies.
func (s *Server) PostAuthLogin(ctx context.Context, request PostAuthLoginRequestObject) (PostAuthLoginResponseObject, error) {
	if request.Body == nil {
		return PostAuthLogin400JSONResponse{Error: "request body is required"}, nil
	}
	username := strings.TrimSpace(request.Body.Username)
	password := request.Body.Password
	if username == "" || password == "" {
		return PostAuthLogin400JSONResponse{Error: "username and password are required"}, nil
	}

	user, err := s.users.UserByUsername(ctx, username)
	if err != nil && !errors.Is(err, store.ErrUserNotFound) {
		return nil, err
	}
	// Run the bcrypt comparison even when the user is missing so the response
	// time does not reveal whether the username exists.
	if errors.Is(err, store.ErrUserNotFound) || !auth.CheckPassword(user.PasswordHash, password) {
		return PostAuthLogin401JSONResponse{Error: "invalid username or password"}, nil
	}

	resp, err := s.startSession(ctx, user.User, http.StatusOK)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// PostAuthLogout deletes the current session and clears the cookies. The
// session and CSRF checks are enforced by the auth middleware before this runs.
func (s *Server) PostAuthLogout(ctx context.Context, _ PostAuthLogoutRequestObject) (PostAuthLogoutResponseObject, error) {
	if sess, ok := sessionFrom(ctx); ok {
		if err := s.sessions.DeleteSession(ctx, sess.ID); err != nil {
			return nil, err
		}
	}

	r := requestFrom(ctx)
	return logoutResponse{cookies: []*http.Cookie{
		s.sessionCookie(r, "", -1),
		s.csrfCookie(r, "", -1),
	}}, nil
}

// GetAuthMe returns the user bound to the current session. The session check is
// enforced by the auth middleware before this runs.
func (s *Server) GetAuthMe(ctx context.Context, _ GetAuthMeRequestObject) (GetAuthMeResponseObject, error) {
	sess, ok := sessionFrom(ctx)
	if !ok {
		return GetAuthMe401JSONResponse{Error: "authentication required"}, nil
	}

	body, err := userResponse(store.User{ID: sess.UserID, Username: sess.Username, Role: sess.Role})
	if err != nil {
		return nil, err
	}
	return GetAuthMe200JSONResponse(body), nil
}

// startSession rotates the caller's session (deleting any prior session
// referenced by the request cookie), mints a new one, and builds the success
// response carrying the session and CSRF cookies.
func (s *Server) startSession(ctx context.Context, user store.User, statusCode int) (authSuccessResponse, error) {
	r := requestFrom(ctx)
	s.revokeRequestSession(ctx, r)

	sess, err := s.sessions.CreateSession(ctx, auth.Identity{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
	if err != nil {
		return authSuccessResponse{}, err
	}

	body, err := userResponse(user)
	if err != nil {
		return authSuccessResponse{}, err
	}

	maxAge := int(s.authSettings.SessionAbsoluteTTL.Seconds())
	return authSuccessResponse{
		statusCode: statusCode,
		cookies: []*http.Cookie{
			s.sessionCookie(r, sess.ID, maxAge),
			s.csrfCookie(r, sess.CSRFToken, maxAge),
		},
		body: body,
	}, nil
}

// revokeRequestSession best-effort deletes the session referenced by the
// request's session cookie, so logging in rotates away any stale session.
func (s *Server) revokeRequestSession(ctx context.Context, r *http.Request) {
	if r == nil {
		return
	}
	c, err := r.Cookie(s.authSettings.SessionCookieName)
	if err != nil || c.Value == "" {
		return
	}
	_ = s.sessions.DeleteSession(ctx, c.Value)
}

// sessionCookie builds the HttpOnly session cookie. A maxAge of -1 clears it.
func (s *Server) sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     s.authSettings.SessionCookieName,
		Value:    value,
		Path:     basePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   requestScheme(r) == "https",
		SameSite: http.SameSiteLaxMode,
	}
}

// csrfCookie builds the readable (non-HttpOnly) CSRF cookie so the SPA can
// echo its value in the X-CSRF-Token header. A maxAge of -1 clears it.
func (s *Server) csrfCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     s.authSettings.CSRFCookieName,
		Value:    value,
		Path:     basePath,
		MaxAge:   maxAge,
		HttpOnly: false,
		Secure:   requestScheme(r) == "https",
		SameSite: http.SameSiteLaxMode,
	}
}

// userResponse maps a store user to the API response shape.
func userResponse(u store.User) (UserResponse, error) {
	id, err := uuid.Parse(u.ID)
	if err != nil {
		return UserResponse{}, fmt.Errorf("httpapi: parse user id %q: %w", u.ID, err)
	}
	return UserResponse{
		Id:       id,
		Username: u.Username,
		Role:     UserResponseRole(u.Role),
	}, nil
}

// authSuccessResponse carries the cookies and user body returned by register
// and login. It satisfies both PostAuthRegisterResponseObject and
// PostAuthLoginResponseObject so the two flows share one implementation.
type authSuccessResponse struct {
	statusCode int
	cookies    []*http.Cookie
	body       UserResponse
}

func (resp authSuccessResponse) write(w http.ResponseWriter) error {
	for _, c := range resp.cookies {
		http.SetCookie(w, c)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.statusCode)
	return json.NewEncoder(w).Encode(resp.body)
}

func (resp authSuccessResponse) VisitPostAuthRegisterResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

func (resp authSuccessResponse) VisitPostAuthLoginResponse(w http.ResponseWriter) error {
	return resp.write(w)
}

// logoutResponse clears the auth cookies and returns 204 No Content.
type logoutResponse struct {
	cookies []*http.Cookie
}

func (resp logoutResponse) VisitPostAuthLogoutResponse(w http.ResponseWriter) error {
	for _, c := range resp.cookies {
		http.SetCookie(w, c)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
