package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/vancanhuit/devopsbin/internal/store/sqlc"
)

// uniqueViolationCode is the PostgreSQL SQLSTATE for a unique-constraint
// violation, used to map a duplicate username to ErrUsernameTaken.
const uniqueViolationCode = "23505"

// starterAccountName and starterAccountBalance configure the account opened for
// a newly registered user so the transfer demo is usable immediately. They
// mirror the seeded demo accounts.
const (
	starterAccountName    = "Checking"
	starterAccountBalance = 100000
)

// ErrUsernameTaken is returned by RegisterUser when the username already exists.
var ErrUsernameTaken = errors.New("store: username already taken")

// ErrUserNotFound is returned by UserByUsername when no user matches.
var ErrUserNotFound = errors.New("store: user not found")

// User is the domain view of a user exposed to callers, with the UUID rendered
// as a canonical string and without the password hash.
type User struct {
	ID       string
	Username string
	Role     string
}

// UserWithHash augments User with the stored bcrypt password hash for use by
// the login flow.
type UserWithHash struct {
	User
	PasswordHash string
}

// NewUser holds the fields required to register a user.
type NewUser struct {
	Username     string
	PasswordHash string
	Role         string
}

// RegisterUser creates a user and a starter account in a single transaction. A
// duplicate username maps to ErrUsernameTaken; any other failure is returned
// wrapped. On success it returns the created user.
func (s *Store) RegisterUser(ctx context.Context, params NewUser) (User, error) {
	var created sqlc.CreateUserRow
	err := s.WithTx(ctx, func(q *sqlc.Queries) error {
		row, err := q.CreateUser(ctx, sqlc.CreateUserParams{
			Username:     params.Username,
			PasswordHash: params.PasswordHash,
			Role:         params.Role,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrUsernameTaken
			}
			return fmt.Errorf("store: create user: %w", err)
		}

		if _, err := q.CreateAccount(ctx, sqlc.CreateAccountParams{
			UserID:       row.ID,
			Name:         starterAccountName,
			BalanceCents: starterAccountBalance,
		}); err != nil {
			return fmt.Errorf("store: create starter account: %w", err)
		}

		created = row
		return nil
	})
	if err != nil {
		return User{}, err
	}

	return User{
		ID:       uuidString(created.ID),
		Username: created.Username,
		Role:     created.Role,
	}, nil
}

// UserByUsername looks up a user (with password hash) by username. A missing
// user maps to ErrUserNotFound.
func (s *Store) UserByUsername(ctx context.Context, username string) (UserWithHash, error) {
	row, err := s.Queries().GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserWithHash{}, ErrUserNotFound
		}
		return UserWithHash{}, fmt.Errorf("store: get user by username: %w", err)
	}

	return UserWithHash{
		User: User{
			ID:       uuidString(row.ID),
			Username: row.Username,
			Role:     row.Role,
		},
		PasswordHash: row.PasswordHash,
	}, nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

// uuidString renders a pgtype.UUID as its canonical 8-4-4-4-12 hyphenated form.
// An invalid (NULL) UUID renders as the empty string.
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
