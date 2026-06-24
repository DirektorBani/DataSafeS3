-- Tenant groups: named bucket access collections within a tenant

CREATE TABLE IF NOT EXISTS tenant_groups (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    access_level TEXT NOT NULL DEFAULT 'read',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS tenant_group_buckets (
    group_id    TEXT NOT NULL REFERENCES tenant_groups (id) ON DELETE CASCADE,
    bucket_key  TEXT NOT NULL REFERENCES buckets (storage_key) ON DELETE CASCADE,
    PRIMARY KEY (group_id, bucket_key)
);
CREATE INDEX IF NOT EXISTS idx_tenant_group_buckets_bucket ON tenant_group_buckets (bucket_key);

CREATE TABLE IF NOT EXISTS tenant_group_members (
    group_id  TEXT NOT NULL REFERENCES tenant_groups (id) ON DELETE CASCADE,
    user_id   TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_group_members_user ON tenant_group_members (user_id);
