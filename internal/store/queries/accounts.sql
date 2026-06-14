-- name: CreateAccount :one
INSERT INTO accounts (user_id, name, balance_cents)
VALUES ($1, $2, $3)
RETURNING id, user_id, name, balance_cents, created_at, updated_at;

-- name: GetAccountByID :one
SELECT id, user_id, name, balance_cents, created_at, updated_at
FROM accounts
WHERE id = $1;

-- name: ListAccountsByUser :many
SELECT id, user_id, name, balance_cents, created_at, updated_at
FROM accounts
WHERE user_id = $1
ORDER BY id;
