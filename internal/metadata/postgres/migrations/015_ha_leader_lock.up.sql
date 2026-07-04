CREATE TABLE IF NOT EXISTS ha_leader_lock (
    lock_id TEXT PRIMARY KEY DEFAULT 'storage-server',
    holder_id TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
