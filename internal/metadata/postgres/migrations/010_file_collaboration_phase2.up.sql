-- Phase 2 file collaboration: prefix grants, notifications, recent items

CREATE TABLE IF NOT EXISTS bucket_prefix_access_grants (
    bucket_key TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    prefix     TEXT NOT NULL,
    can_read   BOOLEAN NOT NULL DEFAULT TRUE,
    can_write  BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (bucket_key, user_id, prefix)
);

CREATE TABLE IF NOT EXISTS user_notifications (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    kind       TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    link       TEXT NOT NULL DEFAULT '',
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_notifications_user ON user_notifications (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS recent_items (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    bucket      TEXT NOT NULL,
    prefix      TEXT NOT NULL DEFAULT '',
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_recent_items_user ON recent_items (user_id, accessed_at DESC);
