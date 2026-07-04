ALTER TABLE federation_clusters ADD COLUMN IF NOT EXISTS cluster_id TEXT NOT NULL DEFAULT 'local';

UPDATE federation_clusters SET cluster_id = 'local' WHERE cluster_id IS NULL OR cluster_id = '';

CREATE INDEX IF NOT EXISTS idx_federation_clusters_cluster_id ON federation_clusters (cluster_id);
