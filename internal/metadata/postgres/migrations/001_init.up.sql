CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id              TEXT PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    email           TEXT NOT NULL DEFAULT '',
    password_hash   TEXT NOT NULL DEFAULT '',
    role            TEXT NOT NULL DEFAULT 'user',
    status          TEXT NOT NULL DEFAULT 'active',
    tenant_id       TEXT REFERENCES tenants(id),
    mfa_enabled     BOOLEAN NOT NULL DEFAULT FALSE,
    totp_secret     TEXT,
    recovery_codes  JSONB,
    auth_source     TEXT,
    max_size_bytes  BIGINT,
    max_objects     BIGINT,
    last_login      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_username_trgm ON users USING gin (username gin_trgm_ops);

CREATE TABLE IF NOT EXISTS access_keys (
    access_key  TEXT PRIMARY KEY,
    secret_key  TEXT NOT NULL,
    label       TEXT,
    owner_id    TEXT,
    owner       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_access_keys_owner ON access_keys(owner);
CREATE INDEX IF NOT EXISTS idx_access_keys_key ON access_keys(access_key);

CREATE TABLE IF NOT EXISTS buckets (
    name                  TEXT PRIMARY KEY,
    owner                 TEXT NOT NULL,
    tenant_id             TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    policy                TEXT,
    lifecycle_rules       JSONB,
    description           TEXT,
    versioning_enabled    BOOLEAN NOT NULL DEFAULT FALSE,
    versioning_suspended  BOOLEAN NOT NULL DEFAULT FALSE,
    object_lock_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    retention_days        INT,
    retention_mode        TEXT,
    storage_class         TEXT,
    visibility            TEXT,
    max_size_bytes        BIGINT,
    max_objects           BIGINT,
    tags                  JSONB
);
CREATE INDEX IF NOT EXISTS idx_buckets_tenant ON buckets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_buckets_owner ON buckets(owner);
CREATE INDEX IF NOT EXISTS idx_buckets_name_trgm ON buckets USING gin (name gin_trgm_ops);

CREATE TABLE IF NOT EXISTS objects (
    bucket              TEXT NOT NULL REFERENCES buckets(name) ON DELETE CASCADE,
    key                 TEXT NOT NULL,
    version_id          TEXT NOT NULL DEFAULT '',
    size                BIGINT NOT NULL DEFAULT 0,
    etag                TEXT,
    content_type        TEXT,
    last_modified       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    scheduled_delete_at TIMESTAMPTZ,
    is_delete_marker    BOOLEAN NOT NULL DEFAULT FALSE,
    legal_hold          BOOLEAN NOT NULL DEFAULT FALSE,
    retention_until     TIMESTAMPTZ,
    storage_class       TEXT,
    metadata            JSONB,
    tags                JSONB,
    created_at          TIMESTAMPTZ,
    is_latest           BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (bucket, key, version_id)
);
CREATE INDEX IF NOT EXISTS idx_objects_bucket_key ON objects(bucket, key);
CREATE INDEX IF NOT EXISTS idx_objects_prefix ON objects(bucket, key text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_objects_key_trgm ON objects USING gin (key gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_objects_latest ON objects(bucket, key) WHERE is_latest = TRUE;
CREATE INDEX IF NOT EXISTS idx_objects_version ON objects(version_id);

CREATE TABLE IF NOT EXISTS multipart_uploads (
    upload_id   TEXT PRIMARY KEY,
    bucket      TEXT NOT NULL,
    key         TEXT NOT NULL,
    initiated   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_multipart_bucket ON multipart_uploads(bucket);

CREATE TABLE IF NOT EXISTS audit_logs (
    id              TEXT PRIMARY KEY,
    ts              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    username        TEXT,
    action          TEXT NOT NULL,
    resource_type   TEXT,
    resource_name   TEXT,
    ip_address      TEXT
);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_logs(ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(username);

CREATE TABLE IF NOT EXISTS webhooks (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    events      JSONB NOT NULL DEFAULT '[]',
    headers     JSONB,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id            TEXT PRIMARY KEY,
    webhook_id    TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event         TEXT NOT NULL,
    url           TEXT NOT NULL,
    status_code   INT,
    success       BOOLEAN NOT NULL DEFAULT FALSE,
    error         TEXT,
    attempts      INT NOT NULL DEFAULT 0,
    payload       TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_attempt  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_wh ON webhook_deliveries(webhook_id, created_at DESC);

CREATE TABLE IF NOT EXISTS system_config (
    id      TEXT PRIMARY KEY DEFAULT 'system',
    data    JSONB NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS trash (
    id              TEXT PRIMARY KEY,
    original_bucket TEXT NOT NULL,
    original_key    TEXT NOT NULL,
    trash_key       TEXT NOT NULL,
    size            BIGINT NOT NULL DEFAULT 0,
    version_id      TEXT,
    deleted_by      TEXT,
    deleted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_tokens (
    id          TEXT PRIMARY KEY,
    name        TEXT,
    token_hash  TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    username    TEXT,
    scopes      JSONB,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);

CREATE TABLE IF NOT EXISTS favorites (
    id          TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    type        TEXT NOT NULL,
    bucket      TEXT NOT NULL,
    prefix      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, id)
);

CREATE TABLE IF NOT EXISTS usage_counters (
    id          TEXT PRIMARY KEY DEFAULT 'global',
    upload      BIGINT NOT NULL DEFAULT 0,
    download    BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS usage_snapshots (
    day             TEXT PRIMARY KEY,
    storage_bytes   BIGINT NOT NULL DEFAULT 0,
    object_count    INT NOT NULL DEFAULT 0,
    bucket_count    INT NOT NULL DEFAULT 0,
    upload_bytes    BIGINT NOT NULL DEFAULT 0,
    download_bytes  BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS gateway_connections (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    endpoint    TEXT NOT NULL,
    region      TEXT,
    access_key  TEXT,
    secret_key  TEXT,
    path_style  BOOLEAN NOT NULL DEFAULT TRUE,
    tls_verify  BOOLEAN NOT NULL DEFAULT TRUE,
    status      TEXT,
    last_check  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS replication_rules (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    source_bucket       TEXT NOT NULL,
    dest_connection_id  TEXT NOT NULL,
    dest_bucket         TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS replication_tasks (
    id              TEXT PRIMARY KEY,
    rule_id         TEXT NOT NULL,
    event           TEXT NOT NULL,
    source_bucket   TEXT NOT NULL,
    key             TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INT NOT NULL DEFAULT 0,
    bytes           BIGINT NOT NULL DEFAULT 0,
    error           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    next_attempt    TIMESTAMPTZ,
    processed_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_repl_tasks_status ON replication_tasks(status, next_attempt);

CREATE TABLE IF NOT EXISTS replication_errors (
    id              TEXT PRIMARY KEY,
    task_id         TEXT,
    rule_id         TEXT NOT NULL,
    event           TEXT NOT NULL,
    source_bucket   TEXT NOT NULL,
    key             TEXT NOT NULL,
    message         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gateway_stats (
    id                      TEXT PRIMARY KEY DEFAULT 'global',
    pending_count           INT NOT NULL DEFAULT 0,
    bytes_replicated        BIGINT NOT NULL DEFAULT 0,
    replication_errors      INT NOT NULL DEFAULT 0,
    oldest_pending          TIMESTAMPTZ,
    last_processed_at       TIMESTAMPTZ,
    tasks_completed_total   INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sync_jobs (
    id              TEXT PRIMARY KEY,
    rule_id         TEXT NOT NULL,
    status          TEXT NOT NULL,
    objects_synced  INT NOT NULL DEFAULT 0,
    errors          INT NOT NULL DEFAULT 0,
    message         TEXT,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS federation_clusters (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    endpoint    TEXT NOT NULL,
    region      TEXT,
    status      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty   BOOLEAN NOT NULL DEFAULT FALSE
);

INSERT INTO tenants (id, name, status) VALUES ('default', 'Default', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO usage_counters (id) VALUES ('global') ON CONFLICT DO NOTHING;
INSERT INTO gateway_stats (id) VALUES ('global') ON CONFLICT DO NOTHING;
INSERT INTO system_config (id, data) VALUES ('system', '{}') ON CONFLICT DO NOTHING;
