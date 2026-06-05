-- notification-service initial schema.
-- A notification is a user-facing message projected from domain events
-- (order filled, deposit confirmed, position liquidated, …).
CREATE TABLE IF NOT EXISTS notifications (
    id         bigserial   PRIMARY KEY,
    user_id    bigint      NOT NULL,
    type       text        NOT NULL,
    title      text        NOT NULL,
    message    text        NOT NULL,
    pair       text        NOT NULL DEFAULT '',
    is_read    boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Hot reads are "this user's notifications, newest first" and "unread count".
-- A composite index on (user_id, is_read) serves the unread filter; created_at
-- keyset ordering is covered by the per-user scan being small.
CREATE INDEX IF NOT EXISTS idx_notif_user_read ON notifications (user_id, is_read);
CREATE INDEX IF NOT EXISTS idx_notif_user_created ON notifications (user_id, created_at DESC);
