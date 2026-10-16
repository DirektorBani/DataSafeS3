-- Access security (v1.6 E00). Additive: can_read/can_write stay for a 1.5.0 rollback.

ALTER TABLE bucket_access_grants ADD COLUMN IF NOT EXISTS level TEXT NOT NULL DEFAULT 'read';
UPDATE bucket_access_grants SET level = 'write' WHERE can_write = TRUE AND level = 'read';

CREATE TABLE IF NOT EXISTS sharing_policies (
    scope TEXT NOT NULL,
    scope_id TEXT NOT NULL DEFAULT '',
    public_access_blocked BOOLEAN,
    share_links_allowed BOOLEAN,
    presign_allowed BOOLEAN,
    max_share_ttl_sec BIGINT,
    max_presign_ttl_sec BIGINT,
    share_password_required BOOLEAN,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (scope, scope_id)
);

ALTER TABLE shared_links ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE shared_links ADD COLUMN IF NOT EXISTS blocked_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE shared_links ADD COLUMN IF NOT EXISTS password_failures INT NOT NULL DEFAULT 0;
ALTER TABLE shared_links ADD COLUMN IF NOT EXISTS password_failure_at TIMESTAMPTZ;

ALTER TABLE access_keys ADD COLUMN IF NOT EXISTS created_with_mfa BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_policy_since TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS bucket_admin_since TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_step BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS mfa_action_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    nonce TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mfa_action_tokens_expires ON mfa_action_tokens (expires_at);
