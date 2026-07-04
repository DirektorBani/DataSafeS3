ALTER TABLE site_replication_rules DROP CONSTRAINT IF EXISTS site_repl_rules_peer_or_cluster;
DROP INDEX IF EXISTS idx_site_repl_rules_trusted_cluster;
ALTER TABLE site_replication_rules DROP COLUMN IF EXISTS trusted_cluster_id;
-- Existing AK/SK peers must have peer_id; no trusted-only rules expected on downgrade.
ALTER TABLE site_replication_rules ALTER COLUMN peer_id SET NOT NULL;
