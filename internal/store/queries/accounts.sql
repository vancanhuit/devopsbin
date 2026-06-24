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

-- name: ListAllAccounts :many
SELECT a.id, u.username AS owner_username, a.name, a.balance_cents, a.created_at
FROM accounts a
JOIN users u ON u.id = a.user_id
ORDER BY u.username, a.id;

-- name: GetAccountsForUpdate :many
-- Locks the requested accounts FOR UPDATE in a deterministic id order so that
-- concurrent transfers touching the same pair cannot deadlock on lock ordering.
SELECT id, user_id, balance_cents
FROM accounts
WHERE id = ANY(@ids::uuid[])
ORDER BY id
FOR UPDATE;

-- name: AdjustBalance :one
-- Applies a signed delta to an account balance and returns the new balance.
UPDATE accounts
SET balance_cents = balance_cents + @delta_cents,
    updated_at = now()
WHERE id = @id
RETURNING balance_cents;
