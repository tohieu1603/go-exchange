-- name: UpsertUserTradePair :one
-- Increment the cross-user trade counter for (user1<user2, pair); the conflict
-- path bumps trade_count and adds the volume. Returns the post-update counters
-- the fraud detector thresholds against.
INSERT INTO user_trade_pairs (user1_id, user2_id, pair, trade_count, total_vol, first_trade, last_trade)
VALUES (@user1_id, @user2_id, @pair, 1, @total::numeric, now(), now())
ON CONFLICT (user1_id, user2_id, pair)
DO UPDATE SET trade_count = user_trade_pairs.trade_count + 1,
              total_vol = user_trade_pairs.total_vol + EXCLUDED.total_vol,
              last_trade = now()
RETURNING trade_count, total_vol, first_trade;

-- name: CountFraudByTypeUsersActive :one
-- Existing-flag guard for flagBonusFarming (ignores already-dismissed flags).
SELECT count(*) FROM fraud_logs
WHERE fraud_type = $1 AND user_ids = $2 AND action <> 'DISMISSED';

-- name: CountFraudByTypeUsers :one
SELECT count(*) FROM fraud_logs WHERE fraud_type = $1 AND user_ids = $2;

-- name: CreateFraudLog :one
INSERT INTO fraud_logs (user_ids, fraud_type, description, evidence, action, admin_note)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at;

-- name: ListFraudLogs :many
-- An empty @search disables the free-text filter.
SELECT id, user_ids, fraud_type, description, evidence, action, admin_note, created_at
FROM fraud_logs
WHERE (@search::text = '' OR user_ids ILIKE '%' || @search::text || '%'
       OR fraud_type ILIKE '%' || @search::text || '%'
       OR description ILIKE '%' || @search::text || '%')
ORDER BY created_at DESC LIMIT @lim OFFSET @off;

-- name: CountFraudLogs :one
SELECT count(*) FROM fraud_logs
WHERE (@search::text = '' OR user_ids ILIKE '%' || @search::text || '%'
       OR fraud_type ILIKE '%' || @search::text || '%'
       OR description ILIKE '%' || @search::text || '%');

-- name: UpdateFraudAction :exec
UPDATE fraud_logs SET action = $2, admin_note = $3 WHERE id = $1;
