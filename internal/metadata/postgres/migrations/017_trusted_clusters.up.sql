-- Multi-cluster trusted replication (mTLS pairing, CRL metadata)

CREATE TABLE IF NOT EXISTS trusted_clusters (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    auth_mode TEXT NOT NULL DEFAULT 'mtls',
    peer_ca_fingerprint TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'healthy',
    cert_expires_at TIMESTAMPTZ,
    last_rotated_at TIMESTAMPTZ,
    next_rotation_at TIMESTAMPTZ,
    safety_number TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    is_local BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_trusted_clusters_active ON trusted_clusters (active) WHERE active = TRUE;

CREATE TABLE IF NOT EXISTS cluster_pairing_sessions (
    token_hash TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    initiator_cluster_id TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cluster_pairing_sessions_expires ON cluster_pairing_sessions (expires_at);

CREATE TABLE IF NOT EXISTS cluster_certificates (
    serial TEXT PRIMARY KEY,
    cluster_id TEXT NOT NULL REFERENCES trusted_clusters(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    not_before TIMESTAMPTZ NOT NULL,
    not_after TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_cluster_certificates_cluster ON cluster_certificates (cluster_id);

-- site_replication_peers.secret_key remains; application layer encrypts via fieldenc on write (PathSiteReplSecretKey).
