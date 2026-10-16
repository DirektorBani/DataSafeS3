ALTER TABLE multipart_uploads DROP COLUMN IF EXISTS encryption;
ALTER TABLE objects DROP COLUMN IF EXISTS encryption;
DROP TABLE IF EXISTS cluster_node_heartbeats;
DROP TABLE IF EXISTS kms_state;
DROP TABLE IF EXISTS kms_keys;
DROP TABLE IF EXISTS bucket_encryption;
