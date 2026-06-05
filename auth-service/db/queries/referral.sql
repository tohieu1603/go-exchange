-- name: CreateReferralCode :one
INSERT INTO referral_codes (user_id, code, is_default, usage_count)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at;

-- name: GetReferralCodeByValue :one
SELECT id, user_id, code, is_default, usage_count, created_at FROM referral_codes WHERE code = $1;

-- name: GetDefaultReferralCode :one
SELECT id, user_id, code, is_default, usage_count, created_at FROM referral_codes WHERE user_id = $1 AND is_default = true;

-- name: IncrementReferralUsage :exec
UPDATE referral_codes SET usage_count = usage_count + 1 WHERE id = $1;

-- name: CreateReferral :one
INSERT INTO referrals (referrer_id, referee_id, code, tier)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at;

-- name: GetReferralByReferee :one
SELECT id, referrer_id, referee_id, code, tier, created_at FROM referrals WHERE referee_id = $1;

-- name: ListReferees :many
SELECT id, referrer_id, referee_id, code, tier, created_at
FROM referrals WHERE referrer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountReferees :one
SELECT count(*) FROM referrals WHERE referrer_id = $1;

-- name: CreateReferralCommission :one
-- Idempotent on trade_id; the conflict path returns the existing row's id.
INSERT INTO referral_commissions (referrer_id, referee_id, trade_id, currency, fee_amount, rate, commission)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (trade_id) DO UPDATE SET trade_id = referral_commissions.trade_id
RETURNING id, created_at;

-- name: GetReferralCommissionByTrade :one
SELECT id, referrer_id, referee_id, trade_id, currency, fee_amount, rate, commission, created_at
FROM referral_commissions WHERE trade_id = $1;

-- name: SumCommissionByUser :one
SELECT COALESCE(SUM(commission), 0)::numeric FROM referral_commissions WHERE referrer_id = $1;

-- name: ListCommissions :many
SELECT id, referrer_id, referee_id, trade_id, currency, fee_amount, rate, commission, created_at
FROM referral_commissions WHERE referrer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountCommissions :one
SELECT count(*) FROM referral_commissions WHERE referrer_id = $1;
