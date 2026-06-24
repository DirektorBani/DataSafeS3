-- List performance indexes for cursor pagination (WS-5)
CREATE INDEX IF NOT EXISTS idx_objects_list_cursor
  ON objects (bucket, key)
  WHERE is_latest = TRUE AND is_delete_marker = FALSE;

CREATE INDEX IF NOT EXISTS idx_objects_bucket_prefix_latest
  ON objects (bucket, key text_pattern_ops)
  WHERE is_latest = TRUE AND is_delete_marker = FALSE;

-- STS session token support (WS-4)
ALTER TABLE access_keys ADD COLUMN IF NOT EXISTS session_token TEXT;
ALTER TABLE access_keys ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
