ALTER TABLE cluster_node_heartbeats DROP COLUMN IF EXISTS kms_checked_at;
ALTER TABLE cluster_node_heartbeats DROP COLUMN IF EXISTS kms_error;
ALTER TABLE cluster_node_heartbeats DROP COLUMN IF EXISTS kms_available;
ALTER TABLE cluster_node_heartbeats DROP COLUMN IF EXISTS kms_provider;
