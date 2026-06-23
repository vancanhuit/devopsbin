-- name: ListTransfers :many
SELECT
    t.id,
    t.from_account_id,
    fa.name AS from_account_name,
    t.to_account_id,
    ta.name AS to_account_name,
    t.amount_cents,
    t.created_at
FROM transfers t
JOIN accounts fa ON fa.id = t.from_account_id
JOIN accounts ta ON ta.id = t.to_account_id
ORDER BY t.created_at DESC, t.id;
