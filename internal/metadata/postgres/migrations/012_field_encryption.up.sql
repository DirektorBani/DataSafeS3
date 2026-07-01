-- Field encryption key registry (KEK metadata for X25519 envelope encryption).
--
-- Encrypted field values stay in existing TEXT/JSONB columns with wire prefix enc:v1:
-- (see docs/specs/field-encryption-1.0.3-tz.md). Private KEK material is never stored here.
--
-- Active key: exactly one row with is_active=TRUE and retired_at IS NULL (enforced in app layer).
-- Decrypt: all rows with retired_at IS NULL; encrypt: only the active key.

CREATE TABLE IF NOT EXISTS encryption_key_registry (
    id          BIGSERIAL PRIMARY KEY,
    kek_id      TEXT NOT NULL UNIQUE,
    algorithm   TEXT NOT NULL DEFAULT 'x25519-aes256-gcm',
    public_key  BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rotated_at  TIMESTAMPTZ,
    retired_at  TIMESTAMPTZ,
    is_active   BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_encryption_key_registry_active
    ON encryption_key_registry (kek_id)
    WHERE is_active = TRUE AND retired_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_encryption_key_registry_decrypt
    ON encryption_key_registry (kek_id)
    WHERE retired_at IS NULL;

COMMENT ON TABLE encryption_key_registry IS
    'Public KEK metadata for application-layer field encryption (X25519 + AES-256-GCM envelope).';

COMMENT ON COLUMN encryption_key_registry.kek_id IS
    'Stable operator-chosen identifier referenced in enc:v1:<kek_id>:... ciphertext blobs.';

COMMENT ON COLUMN encryption_key_registry.public_key IS
    'Raw 32-byte X25519 public key (not PEM). Used for registry audit and ECDH verification.';

COMMENT ON COLUMN encryption_key_registry.is_active IS
    'When true, new encrypt operations use this key. At most one active non-retired key.';

COMMENT ON COLUMN encryption_key_registry.retired_at IS
    'When set, decrypt with this kek_id is rejected (post-rotation cleanup).';
