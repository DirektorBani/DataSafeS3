DROP INDEX IF EXISTS idx_shared_links_token_hash;
ALTER TABLE shared_links DROP COLUMN IF EXISTS token_hash;
ALTER TABLE shared_links ALTER COLUMN token SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_shared_links_token ON shared_links(token);
