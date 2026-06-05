-- name: CreateNotification :one
-- Insert a notification, returning the persisted row. All values parameterized.
INSERT INTO notifications (user_id, type, title, message, pair, is_read)
VALUES ($1, $2, $3, $4, $5, false)
RETURNING id, user_id, type, title, message, pair, is_read, created_at;

-- name: ListByUser :many
-- Page of a user's notifications, newest first.
SELECT id, user_id, type, title, message, pair, is_read, created_at
FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListUnreadByUser :many
-- Page of a user's UNREAD notifications, newest first.
SELECT id, user_id, type, title, message, pair, is_read, created_at
FROM notifications
WHERE user_id = $1 AND is_read = false
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: CountByUser :one
SELECT count(*) FROM notifications WHERE user_id = $1;

-- name: CountUnreadByUser :one
SELECT count(*) FROM notifications WHERE user_id = $1 AND is_read = false;

-- name: MarkAsRead :exec
-- Mark one notification read, scoped to its owner (an attacker cannot flip
-- another user's row).
UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2;

-- name: MarkAllRead :exec
UPDATE notifications SET is_read = true WHERE user_id = $1 AND is_read = false;
