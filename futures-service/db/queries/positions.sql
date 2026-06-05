-- name: CreatePosition :one
INSERT INTO futures_positions (user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status, created_at, closed_at;

-- name: FindOpenPositionsByUser :many
SELECT id, user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status, created_at, closed_at
FROM futures_positions
WHERE user_id = $1 AND status = 'OPEN'
ORDER BY created_at DESC;

-- name: FindPositionsByUserAndStatus :many
-- An empty @status disables the status filter.
SELECT id, user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status, created_at, closed_at
FROM futures_positions
WHERE user_id = @user_id AND (@status::text = '' OR status = @status::text)
ORDER BY created_at DESC;

-- name: FindAllOpenPositions :many
SELECT id, user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status, created_at, closed_at
FROM futures_positions
WHERE status = 'OPEN';

-- name: FindPositionByIDForUpdate :one
SELECT id, user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status, created_at, closed_at
FROM futures_positions
WHERE id = $1 AND status = $2
FOR UPDATE;

-- name: FindPositionByUserAndIDForUpdate :one
SELECT id, user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status, created_at, closed_at
FROM futures_positions
WHERE id = $1 AND user_id = $2 AND status = $3
FOR UPDATE;

-- name: FindPositionByUserAndID :one
SELECT id, user_id, pair, side, leverage, entry_price, mark_price, size, margin, unrealized_pn_l, liquidation_price, take_profit, stop_loss, status, created_at, closed_at
FROM futures_positions
WHERE id = $1 AND user_id = $2 AND status = $3;

-- name: SavePosition :exec
UPDATE futures_positions
SET pair = $2, side = $3, leverage = $4, entry_price = $5, mark_price = $6, size = $7,
    margin = $8, unrealized_pn_l = $9, liquidation_price = $10, take_profit = $11,
    stop_loss = $12, status = $13, closed_at = $14
WHERE id = $1;

-- name: UpdateTPSL :exec
-- Update only the provided fields (NULL arg leaves the column unchanged), scoped
-- to the owner and only while OPEN.
UPDATE futures_positions
SET take_profit = COALESCE(sqlc.narg('take_profit')::numeric, take_profit),
    stop_loss   = COALESCE(sqlc.narg('stop_loss')::numeric, stop_loss)
WHERE id = @id AND user_id = @user_id AND status = 'OPEN';
