DROP INDEX IF EXISTS idx_federation_clusters_cluster_id;
ALTER TABLE federation_clusters DROP COLUMN IF EXISTS cluster_id;
