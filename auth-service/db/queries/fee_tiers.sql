-- name: ListFeeTiers :many
SELECT id, level, name, min_volume, maker_fee, taker_fee, description
FROM fee_tiers ORDER BY level ASC;

-- name: GetFeeTierByLevel :one
SELECT id, level, name, min_volume, maker_fee, taker_fee, description
FROM fee_tiers WHERE level = $1;

-- name: CountFeeTiers :one
SELECT count(*) FROM fee_tiers;

-- name: InsertFeeTier :exec
INSERT INTO fee_tiers (level, name, min_volume, maker_fee, taker_fee, description)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetUserVolume :one
SELECT user_id, volume, tier_level, updated_at FROM user_volume30ds WHERE user_id = $1;

-- name: UpsertVolume :exec
INSERT INTO user_volume30ds (user_id, volume, tier_level, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (user_id) DO UPDATE SET volume = EXCLUDED.volume, tier_level = EXCLUDED.tier_level, updated_at = now();

-- name: IncrementVolume :exec
INSERT INTO user_volume30ds (user_id, volume, tier_level, updated_at)
VALUES (@user_id, @delta::numeric, 0, now())
ON CONFLICT (user_id) DO UPDATE SET volume = user_volume30ds.volume + EXCLUDED.volume, updated_at = now();
