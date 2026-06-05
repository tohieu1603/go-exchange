-- name: CreateWithdrawal :one
INSERT INTO withdrawals (user_id, amount, currency, bank_code, bank_account, account_name, status, admin_note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, user_id, amount, currency, bank_code, bank_account, account_name, status, admin_note, created_at, updated_at;

-- name: GetWithdrawalByID :one
SELECT id, user_id, amount, currency, bank_code, bank_account, account_name, status, admin_note, created_at, updated_at
FROM withdrawals
WHERE id = $1;

-- name: ListWithdrawalsByUser :many
SELECT id, user_id, amount, currency, bank_code, bank_account, account_name, status, admin_note, created_at, updated_at
FROM withdrawals
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountWithdrawalsByUser :one
SELECT count(*) FROM withdrawals WHERE user_id = $1;

-- name: GetLatestPendingWithdrawal :one
SELECT id, user_id, amount, currency, bank_code, bank_account, account_name, status, admin_note, created_at, updated_at
FROM withdrawals
WHERE user_id = $1 AND status = 'PENDING'
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateWithdrawal :exec
UPDATE withdrawals
SET status = $2, admin_note = $3, updated_at = now()
WHERE id = $1;

-- name: ListWithdrawalsAdmin :many
-- Admin read model: optional status filter and optional free-text search on the
-- owning user's email OR the destination bank account.
SELECT w.id, w.user_id, w.amount, w.currency, w.bank_code, w.bank_account, w.account_name, w.status, w.admin_note, w.created_at, w.updated_at
FROM withdrawals w
WHERE (@status::text = '' OR w.status = @status::text)
  AND (@search::text = '' OR w.bank_account ILIKE '%' || @search::text || '%' OR EXISTS (
        SELECT 1 FROM users u WHERE u.id = w.user_id AND u.email ILIKE '%' || @search::text || '%'))
ORDER BY w.created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountWithdrawalsAdmin :one
SELECT count(*)
FROM withdrawals w
WHERE (@status::text = '' OR w.status = @status::text)
  AND (@search::text = '' OR w.bank_account ILIKE '%' || @search::text || '%' OR EXISTS (
        SELECT 1 FROM users u WHERE u.id = w.user_id AND u.email ILIKE '%' || @search::text || '%'));
