-- name: CreatePromotion :one
INSERT INTO bonus_promotions (name, description, bonus_percent, max_bonus_amount, target_type, target_user_ids, trigger_type, min_deposit, is_active, start_at, end_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, created_at;

-- name: FindActivePromotions :many
SELECT id, name, description, bonus_percent, max_bonus_amount, target_type, target_user_ids, trigger_type, min_deposit, is_active, start_at, end_at, created_at
FROM bonus_promotions WHERE is_active = true;

-- name: FindAllPromotions :many
SELECT id, name, description, bonus_percent, max_bonus_amount, target_type, target_user_ids, trigger_type, min_deposit, is_active, start_at, end_at, created_at
FROM bonus_promotions ORDER BY created_at DESC;

-- name: FindPromotionByID :one
SELECT id, name, description, bonus_percent, max_bonus_amount, target_type, target_user_ids, trigger_type, min_deposit, is_active, start_at, end_at, created_at
FROM bonus_promotions WHERE id = $1;

-- name: UpdatePromotion :exec
UPDATE bonus_promotions SET
    name = $2, description = $3, bonus_percent = $4, max_bonus_amount = $5, target_type = $6,
    target_user_ids = $7, trigger_type = $8, min_deposit = $9, is_active = $10, start_at = $11, end_at = $12
WHERE id = $1;

-- name: CreateUserBonus :one
INSERT INTO user_bonus (user_id, promotion_id, deposit_id, bonus_amount, used_amount, status, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, created_at;

-- name: FindUserBonuses :many
SELECT id, user_id, promotion_id, deposit_id, bonus_amount, used_amount, status, created_at, expires_at
FROM user_bonus WHERE user_id = $1 ORDER BY created_at DESC;

-- name: FindActiveUserBonuses :many
SELECT id, user_id, promotion_id, deposit_id, bonus_amount, used_amount, status, created_at, expires_at
FROM user_bonus WHERE user_id = $1 AND status = 'ACTIVE' ORDER BY created_at ASC;

-- name: UpdateUserBonus :exec
UPDATE user_bonus SET
    user_id = $2, promotion_id = $3, deposit_id = $4, bonus_amount = $5, used_amount = $6, status = $7, expires_at = $8
WHERE id = $1;

-- name: SumActiveBonus :one
SELECT COALESCE(SUM(bonus_amount - used_amount), 0)::numeric
FROM user_bonus WHERE user_id = $1 AND status = 'ACTIVE';
