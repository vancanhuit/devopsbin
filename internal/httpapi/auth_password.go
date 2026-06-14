package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/vancanhuit/devopsbin/internal/auth"
	"github.com/vancanhuit/devopsbin/internal/store"
)

// PostAuthPasswordChange verifies the current password and sets a new one for
// the authenticated user. On success the session is rotated and all of the
// user's other sessions are revoked. The session and CSRF checks are enforced
// by the auth middleware before this runs.
func (s *Server) PostAuthPasswordChange(ctx context.Context, request PostAuthPasswordChangeRequestObject) (PostAuthPasswordChangeResponseObject, error) {
	sess, ok := sessionFrom(ctx)
	if !ok {
		return PostAuthPasswordChange401JSONResponse{Error: "authentication required"}, nil
	}
	if request.Body == nil {
		return PostAuthPasswordChange400JSONResponse{Error: "request body is required"}, nil
	}
	newPassword := request.Body.NewPassword
	if request.Body.CurrentPassword == "" || newPassword == "" {
		return PostAuthPasswordChange400JSONResponse{Error: "current and new passwords are required"}, nil
	}

	user, err := s.users.UserByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return PostAuthPasswordChange401JSONResponse{Error: "authentication required"}, nil
		}
		return nil, err
	}
	if !auth.CheckPassword(user.PasswordHash, request.Body.CurrentPassword) {
		return PostAuthPasswordChange403JSONResponse{Error: "current password is incorrect"}, nil
	}

	hash, err := auth.HashPassword(newPassword, s.authSettings.BcryptCost)
	if err != nil {
		return nil, err
	}
	if err := s.users.UpdatePassword(ctx, user.ID, hash); err != nil {
		return nil, err
	}

	// Rotate the current session to a fresh id/CSRF token, then revoke every
	// other session for the user so other devices are logged out.
	resp, newSession, err := s.startSession(ctx, user.User, http.StatusOK)
	if err != nil {
		return nil, err
	}
	if err := s.sessions.RevokeUserSessions(ctx, user.ID, newSession.ID); err != nil {
		return nil, err
	}
	return passwordChangeResponse{authSuccessResponse: resp}, nil
}

// PostAuthPasswordResetRequest issues a single-use reset token for the given
// username. To avoid leaking which usernames exist, it always returns 200; the
// token is included only when the user exists.
func (s *Server) PostAuthPasswordResetRequest(ctx context.Context, request PostAuthPasswordResetRequestRequestObject) (PostAuthPasswordResetRequestResponseObject, error) {
	if request.Body == nil {
		return PostAuthPasswordResetRequest400JSONResponse{Error: "request body is required"}, nil
	}
	username := strings.TrimSpace(request.Body.Username)
	if username == "" {
		return PostAuthPasswordResetRequest400JSONResponse{Error: "username is required"}, nil
	}

	const ack = "if the user exists, a reset token has been issued"
	if s.recovery == nil {
		return PostAuthPasswordResetRequest200JSONResponse{Message: ack}, nil
	}

	user, err := s.users.UserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return PostAuthPasswordResetRequest200JSONResponse{Message: ack}, nil
		}
		return nil, err
	}

	token, err := s.recovery.CreateResetToken(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return PostAuthPasswordResetRequest200JSONResponse{Message: ack, Token: &token}, nil
}

// PostAuthPasswordReset consumes a single-use reset token and sets a new
// password for the associated user, revoking all of that user's sessions.
func (s *Server) PostAuthPasswordReset(ctx context.Context, request PostAuthPasswordResetRequestObject) (PostAuthPasswordResetResponseObject, error) {
	if request.Body == nil {
		return PostAuthPasswordReset400JSONResponse{Error: "request body is required"}, nil
	}
	if request.Body.Token == "" || request.Body.NewPassword == "" {
		return PostAuthPasswordReset400JSONResponse{Error: "token and new password are required"}, nil
	}
	if s.recovery == nil {
		return PostAuthPasswordReset410JSONResponse{Error: "reset token is invalid or expired"}, nil
	}

	userID, err := s.recovery.ConsumeResetToken(ctx, request.Body.Token)
	if err != nil {
		if errors.Is(err, auth.ErrResetTokenInvalid) {
			return PostAuthPasswordReset410JSONResponse{Error: "reset token is invalid or expired"}, nil
		}
		return nil, err
	}

	hash, err := auth.HashPassword(request.Body.NewPassword, s.authSettings.BcryptCost)
	if err != nil {
		return nil, err
	}
	if err := s.users.UpdatePassword(ctx, userID, hash); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return PostAuthPasswordReset410JSONResponse{Error: "reset token is invalid or expired"}, nil
		}
		return nil, err
	}
	if err := s.sessions.RevokeUserSessions(ctx, userID, ""); err != nil {
		return nil, err
	}
	return PostAuthPasswordReset200JSONResponse{Message: "password has been reset"}, nil
}

// passwordChangeResponse adapts the shared authSuccessResponse (cookies + user
// body) to the password-change operation's response interface.
type passwordChangeResponse struct {
	authSuccessResponse
}

func (resp passwordChangeResponse) VisitPostAuthPasswordChangeResponse(w http.ResponseWriter) error {
	return resp.write(w)
}
