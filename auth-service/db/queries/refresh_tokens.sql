-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, family_id, parent_id, user_agent, ip, issued_at, expires_at, revoked_reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: GetRefreshTokenByHash :one
SELECT id, user_id, token_hash, family_id, parent_id, user_agent, ip, issued_at, expires_at, used_at, revoked_at, revoked_reason
FROM refresh_tokens WHERE token_hash = $1;

-- name: MarkRefreshTokenUsed :exec
UPDATE refresh_tokens SET used_at = now() WHERE id = $1;

-- name: RevokeRefreshTokenFamily :exec
UPDATE refresh_tokens SET revoked_at = now(), revoked_reason = $2
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshTokensByUser :exec
UPDATE refresh_tokens SET revoked_at = now(), revoked_reason = $2
WHERE user_id = $1 AND revoked_at IS NULL;

-- name: RevokeRefreshTokenByID :exec
UPDATE refresh_tokens SET revoked_at = now(), revoked_reason = $2 WHERE id = $1;

-- name: DeleteExpiredRefreshTokens :execrows
-- Retention cleanup: drop tokens that are expired or were revoked before cutoff.
DELETE FROM refresh_tokens
WHERE expires_at < @cutoff OR (revoked_at IS NOT NULL AND revoked_at < @cutoff);
