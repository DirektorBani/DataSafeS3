-- Link site replication rules to trusted clusters (mTLS transport, optional peer_id).
ALTER TABLE site_replication_rules ALTER COLUMN peer_id DROP NOT NULL;

ALTER TABLE site_replication_rules
    ADD COLUMN IF NOT EXISTS trusted_cluster_id TEXT REFERENCES trusted_clusters(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_site_repl_rules_trusted_cluster
    ON site_replication_rules (trusted_cluster_id)
    WHERE trusted_cluster_id IS NOT NULL;

ALTER TABLE site_replication_rules DROP CONSTRAINT IF EXISTS site_repl_rules_peer_or_cluster;
ALTER TABLE site_replication_rules ADD CONSTRAINT site_repl_rules_peer_or_cluster CHECK (
    (peer_id IS NOT NULL AND trusted_cluster_id IS NULL)
    OR (peer_id IS NULL AND trusted_cluster_id IS NOT NULL)
);
