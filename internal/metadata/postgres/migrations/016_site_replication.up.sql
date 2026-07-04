-- Site replication (DataSafeS3 peer)
CREATE TABLE IF NOT EXISTS site_replication_peers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    access_key TEXT NOT NULL,
    secret_key TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS site_replication_rules (
    id TEXT PRIMARY KEY,
    peer_id TEXT NOT NULL REFERENCES site_replication_peers(id) ON DELETE CASCADE,
    source_bucket TEXT NOT NULL,
    dest_bucket TEXT NOT NULL,
    direction TEXT NOT NULL DEFAULT 'one-way',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS site_replication_tasks (
    id TEXT PRIMARY KEY,
    rule_id TEXT NOT NULL REFERENCES site_replication_rules(id) ON DELETE CASCADE,
    event TEXT NOT NULL,
    source_bucket TEXT NOT NULL,
    object_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    bytes BIGINT NOT NULL DEFAULT 0,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    next_attempt TIMESTAMPTZ,
    processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_site_repl_tasks_status ON site_replication_tasks(status, next_attempt);
