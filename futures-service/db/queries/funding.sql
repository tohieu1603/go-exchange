-- name: CreateFundingRate :one
-- Idempotent on (pair, settled_at): a conflict returns the existing row's id via
-- a no-op UPDATE (so the caller always gets an id, matching gorm FirstOrCreate).
INSERT INTO funding_rates (pair, rate, index_price, mark_price, interval, settled_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (pair, settled_at) DO UPDATE SET pair = funding_rates.pair
RETURNING id;

-- name: LatestFundingRate :one
SELECT id, pair, rate, index_price, mark_price, interval, settled_at, created_at
FROM funding_rates
WHERE pair = $1
ORDER BY settled_at DESC
LIMIT 1;

-- name: RecentFundingRates :many
SELECT id, pair, rate, index_price, mark_price, interval, settled_at, created_at
FROM funding_rates
WHERE pair = $1
ORDER BY settled_at DESC
LIMIT $2;

-- name: CreateFundingPayment :one
-- Idempotent on (position_id, funding_rate_id).
INSERT INTO funding_payments (position_id, user_id, funding_rate_id, pair, side, notional, rate, amount)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (position_id, funding_rate_id) DO UPDATE SET position_id = funding_payments.position_id
RETURNING id, created_at;

-- name: FundingHistoryByUser :many
SELECT id, position_id, user_id, funding_rate_id, pair, side, notional, rate, amount, created_at
FROM funding_payments
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountFundingByUser :one
SELECT count(*) FROM funding_payments WHERE user_id = $1;
