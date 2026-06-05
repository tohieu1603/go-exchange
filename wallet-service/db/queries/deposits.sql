-- name: CreateDeposit :one
INSERT INTO deposits (user_id, amount, amount_usdt, exchange_rate, currency, method, status, order_code, qr_code_url, sepay_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, user_id, amount, amount_usdt, exchange_rate, currency, method, status, order_code, qr_code_url, sepay_ref, created_at, updated_at;

-- name: GetDepositByID :one
SELECT id, user_id, amount, amount_usdt, exchange_rate, currency, method, status, order_code, qr_code_url, sepay_ref, created_at, updated_at
FROM deposits
WHERE id = $1;

-- name: GetDepositByOrderCode :one
SELECT id, user_id, amount, amount_usdt, exchange_rate, currency, method, status, order_code, qr_code_url, sepay_ref, created_at, updated_at
FROM deposits
WHERE order_code = $1;

-- name: ListDepositsByUser :many
SELECT id, user_id, amount, amount_usdt, exchange_rate, currency, method, status, order_code, qr_code_url, sepay_ref, created_at, updated_at
FROM deposits
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDepositsByUser :one
SELECT count(*) FROM deposits WHERE user_id = $1;

-- name: UpdateDeposit :exec
-- Persist the mutable fields after a state transition (Confirm/Fail) or once the
-- bank reference is known.
UPDATE deposits
SET status = $2, amount_usdt = $3, exchange_rate = $4, qr_code_url = $5, sepay_ref = $6, updated_at = now()
WHERE id = $1;

-- name: ListDepositsAdmin :many
-- Admin read model: optional status filter and optional free-text search on the
-- owning user's email. An empty string disables the corresponding filter.
SELECT d.id, d.user_id, d.amount, d.amount_usdt, d.exchange_rate, d.currency, d.method, d.status, d.order_code, d.qr_code_url, d.sepay_ref, d.created_at, d.updated_at
FROM deposits d
WHERE (@status::text = '' OR d.status = @status::text)
  AND (@search::text = '' OR EXISTS (
        SELECT 1 FROM users u WHERE u.id = d.user_id AND u.email ILIKE '%' || @search::text || '%'))
ORDER BY d.created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountDepositsAdmin :one
SELECT count(*)
FROM deposits d
WHERE (@status::text = '' OR d.status = @status::text)
  AND (@search::text = '' OR EXISTS (
        SELECT 1 FROM users u WHERE u.id = d.user_id AND u.email ILIKE '%' || @search::text || '%'));
