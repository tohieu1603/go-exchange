-- name: CreateUser :one
INSERT INTO users (email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING id, email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url, created_at, updated_at
FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url, created_at, updated_at
FROM users WHERE id = $1;

-- name: GetUserByGoogleSub :one
SELECT id, email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url, created_at, updated_at
FROM users WHERE google_sub = $1;

-- name: UpdateUser :exec
-- Full-row update (mirrors gorm Save). id/created_at are immutable.
UPDATE users SET
    email = $2, password_hash = $3, full_name = $4, phone = $5, kyc_status = $6,
    is2_fa = $7, two_fa_secret = $8, role = $9, email_verified = $10, kyc_step = $11,
    is_locked = $12, lock_reason = $13, last_login_ip = $14, register_ip = $15,
    google_sub = $16, avatar_url = $17, updated_at = now()
WHERE id = $1;

-- name: UpdateUserKYCStatus :exec
UPDATE users SET kyc_status = $2, updated_at = now() WHERE id = $1;

-- name: CountRealUsers :one
-- Excludes SYSTEM accounts (infrastructure rows), used for demo-seed gating + stats.
SELECT count(*) FROM users WHERE role <> 'SYSTEM';

-- name: CountUsersByKYCStatus :one
SELECT count(*) FROM users WHERE kyc_status = $1;

-- name: ListUsersExclSystem :many
-- userRepo.FindPaginated: customer list (SYSTEM hidden), optional email/name search.
SELECT id, email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url, created_at, updated_at
FROM users
WHERE role <> 'SYSTEM'
  AND (@search::text = '' OR email ILIKE '%' || @search::text || '%' OR full_name ILIKE '%' || @search::text || '%')
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountUsersExclSystem :one
SELECT count(*)
FROM users
WHERE role <> 'SYSTEM'
  AND (@search::text = '' OR email ILIKE '%' || @search::text || '%' OR full_name ILIKE '%' || @search::text || '%');

-- name: ListUsersAdmin :many
-- admin.GetUsers: full list (SYSTEM included), optional search.
SELECT id, email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url, created_at, updated_at
FROM users
WHERE (@search::text = '' OR email ILIKE '%' || @search::text || '%' OR full_name ILIKE '%' || @search::text || '%')
ORDER BY created_at DESC
LIMIT @lim OFFSET @off;

-- name: CountUsersAdmin :one
SELECT count(*)
FROM users
WHERE (@search::text = '' OR email ILIKE '%' || @search::text || '%' OR full_name ILIKE '%' || @search::text || '%');

-- name: ListPendingKYCUsers :many
-- kyc.FindPendingUsers: users awaiting review, newest activity first.
SELECT id, email, password_hash, full_name, phone, kyc_status, is2_fa, two_fa_secret, role, email_verified, kyc_step, is_locked, lock_reason, last_login_ip, register_ip, google_sub, avatar_url, created_at, updated_at
FROM users
WHERE kyc_status = 'PENDING'
ORDER BY updated_at DESC
LIMIT @lim OFFSET @off;

-- name: CountPendingKYCUsers :one
SELECT count(*) FROM users WHERE kyc_status = 'PENDING';

-- name: UserGrowthDaily :many
-- admin chart: new users per day since a cutoff.
SELECT DATE(created_at)::text AS day, count(*) AS count
FROM users
WHERE created_at >= $1
GROUP BY DATE(created_at)
ORDER BY day ASC;
