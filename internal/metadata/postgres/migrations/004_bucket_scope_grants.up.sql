-- Tenant-scoped bucket names and per-bucket access grants

ALTER TABLE buckets ADD COLUMN IF NOT EXISTS storage_key TEXT;
UPDATE buckets SET storage_key = name WHERE storage_key IS NULL OR storage_key = '';
ALTER TABLE buckets ALTER COLUMN storage_key SET NOT NULL;

ALTER TABLE objects DROP CONSTRAINT IF EXISTS objects_bucket_fkey;

ALTER TABLE shared_links DROP CONSTRAINT IF EXISTS shared_links_bucket_fkey;
ALTER TABLE buckets DROP CONSTRAINT IF EXISTS buckets_pkey;
ALTER TABLE buckets ADD PRIMARY KEY (storage_key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_buckets_scope_tenant_name
    ON buckets (tenant_id, name)
    WHERE tenant_id IS NOT NULL AND tenant_id <> '' AND tenant_id <> 'default';

CREATE UNIQUE INDEX IF NOT EXISTS idx_buckets_scope_owner_name
    ON buckets (COALESCE(owner_id, ''), name)
    WHERE tenant_id IS NULL OR tenant_id = '' OR tenant_id = 'default';

ALTER TABLE objects ADD CONSTRAINT objects_bucket_fkey
    FOREIGN KEY (bucket) REFERENCES buckets (storage_key) ON DELETE CASCADE;

ALTER TABLE shared_links ADD CONSTRAINT shared_links_bucket_fkey
    FOREIGN KEY (bucket) REFERENCES buckets (storage_key) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS bucket_access_grants (
    bucket_key  TEXT NOT NULL REFERENCES buckets (storage_key) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    can_read    BOOLEAN NOT NULL DEFAULT TRUE,
    can_write   BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (bucket_key, user_id)
);
CREATE INDEX IF NOT EXISTS idx_bucket_access_grants_user ON bucket_access_grants (user_id);
