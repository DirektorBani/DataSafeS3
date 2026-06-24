DROP INDEX IF EXISTS idx_objects_list_cursor;
DROP INDEX IF EXISTS idx_objects_bucket_prefix_latest;
ALTER TABLE access_keys DROP COLUMN IF EXISTS session_token;
ALTER TABLE access_keys DROP COLUMN IF EXISTS expires_at;
