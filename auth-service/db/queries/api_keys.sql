-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, label, key_id, secret_hash, permissions, ip_whitelist, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, created_at;

-- name: GetAPIKeyByKeyID :one
SELECT id, user_id, label, key_id, secret_hash, permissions, ip_whitelist, last_used_at, last_used_ip, expires_at, revoked_at, created_at
FROM api_keys WHERE key_id = $1 AND revoked_at IS NULL;

-- name: ListAPIKeysByUser :many
SELECT id, user_id, label, key_id, secret_hash, permissions, ip_whitelist, last_used_at, last_used_ip, expires_at, revoked_at, created_at
FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND user_id = $2;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now(), last_used_ip = $2 WHERE id = $1;
