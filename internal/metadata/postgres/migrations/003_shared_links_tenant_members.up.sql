CREATE TABLE IF NOT EXISTS shared_links (
    id              TEXT PRIMARY KEY,
    bucket          TEXT NOT NULL REFERENCES buckets(name) ON DELETE CASCADE,
    key             TEXT NOT NULL,
    token           TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ,
    max_downloads   INT NOT NULL DEFAULT 0,
    download_count  INT NOT NULL DEFAULT 0,
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_shared_links_bucket_key ON shared_links(bucket, key);
CREATE INDEX IF NOT EXISTS idx_shared_links_token ON shared_links(token);

CREATE TABLE IF NOT EXISTS tenant_members (
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',
    PRIMARY KEY (tenant_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_members_user ON tenant_members(user_id);
