-- name: CreateAuditLog :one
INSERT INTO audit_logs (user_id, email, action, outcome, ip, user_agent, device_id, new_device, detail)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, created_at;

-- name: ListAuditByUser :many
SELECT id, user_id, email, action, outcome, ip, user_agent, device_id, new_device, detail, created_at
FROM audit_logs WHERE user_id = $1
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: CountAuditByUser :one
SELECT count(*) FROM audit_logs WHERE user_id = $1;

-- name: ListAuditAll :many
-- An empty @action disables the action filter.
SELECT id, user_id, email, action, outcome, ip, user_agent, device_id, new_device, detail, created_at
FROM audit_logs
WHERE (@action::text = '' OR action = @action::text)
ORDER BY created_at DESC LIMIT @lim OFFSET @off;

-- name: CountAuditAll :one
SELECT count(*) FROM audit_logs WHERE (@action::text = '' OR action = @action::text);

-- name: PruneAuditOlderThan :execrows
DELETE FROM audit_logs WHERE created_at < $1;

-- name: CountAuditDeviceForUser :one
SELECT count(*) FROM audit_logs WHERE user_id = $1 AND device_id = $2;
