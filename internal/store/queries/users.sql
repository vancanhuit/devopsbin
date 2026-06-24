-- name: CreateUser :one
INSERT INTO users (username, password_hash, role)
VALUES ($1, $2, $3)
RETURNING id, username, role, created_at, updated_at;

-- name: GetUserByUsername :one
SELECT id, username, password_hash, role, created_at, updated_at
FROM users
WHERE username = $1;

-- name: GetUserByID :one
SELECT id, username, password_hash, role, created_at, updated_at
FROM users
WHERE id = $1;

-- name: UpdateUserPassword :one
UPDATE users
SET password_hash = $2, updated_at = now()
WHERE id = $1
RETURNING id;

-- name: ListUsers :many
SELECT id, username, role, created_at
FROM users
ORDER BY created_at, id;
