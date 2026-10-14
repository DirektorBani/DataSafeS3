CREATE TABLE IF NOT EXISTS bucket_encryption (
    storage_key TEXT PRIMARY KEY REFERENCES buckets (storage_key) ON DELETE CASCADE,
    algorithm   TEXT NOT NULL,
    kms_key_id  TEXT NOT NULL DEFAULT '',
    provider    TEXT NOT NULL DEFAULT 'local',
    key_status  TEXT NOT NULL DEFAULT 'active'
);

CREATE TABLE IF NOT EXISTS kms_keys (
    id          TEXT PRIMARY KEY,
    provider    TEXT NOT NULL,
    status      TEXT NOT NULL,
    key_version INT NOT NULL DEFAULT 1,
    generation  BIGINT NOT NULL DEFAULT 1,
    tenant_id   TEXT NOT NULL DEFAULT '',
    bucket_key  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS kms_state (
    id                  TEXT PRIMARY KEY DEFAULT 'local',
    fingerprint         TEXT NOT NULL DEFAULT '',
    legacy_sse_objects  BOOLEAN NOT NULL DEFAULT FALSE,
    initialized         BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS cluster_node_heartbeats (
    node_id TEXT PRIMARY KEY,
    version TEXT NOT NULL,
    seen_at TIMESTAMPTZ NOT NULL
);

ALTER TABLE objects ADD COLUMN IF NOT EXISTS encryption JSONB;
ALTER TABLE multipart_uploads ADD COLUMN IF NOT EXISTS encryption JSONB;
