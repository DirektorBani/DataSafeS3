CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE shared_links ADD COLUMN IF NOT EXISTS token_hash TEXT;

UPDATE shared_links
SET token_hash = encode(digest(token, 'sha256'), 'hex')
WHERE token IS NOT NULL AND token != '' AND (token_hash IS NULL OR token_hash = '');

CREATE UNIQUE INDEX IF NOT EXISTS idx_shared_links_token_hash ON shared_links(token_hash)
WHERE token_hash IS NOT NULL AND token_hash != '';

ALTER TABLE shared_links DROP CONSTRAINT IF EXISTS shared_links_token_key;
DROP INDEX IF EXISTS idx_shared_links_token;

ALTER TABLE shared_links ALTER COLUMN token DROP NOT NULL;

UPDATE shared_links SET token = NULL WHERE token_hash IS NOT NULL AND token_hash != '';
