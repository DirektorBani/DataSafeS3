DROP TABLE IF EXISTS mfa_action_tokens;

ALTER TABLE users DROP COLUMN IF EXISTS totp_last_step;
ALTER TABLE users DROP COLUMN IF EXISTS bucket_admin_since;
ALTER TABLE users DROP COLUMN IF EXISTS mfa_policy_since;

ALTER TABLE access_keys DROP COLUMN IF EXISTS created_with_mfa;

ALTER TABLE shared_links DROP COLUMN IF EXISTS password_failure_at;
ALTER TABLE shared_links DROP COLUMN IF EXISTS password_failures;
ALTER TABLE shared_links DROP COLUMN IF EXISTS blocked_reason;
ALTER TABLE shared_links DROP COLUMN IF EXISTS password_hash;

DROP TABLE IF EXISTS sharing_policies;

ALTER TABLE bucket_access_grants DROP COLUMN IF EXISTS level;
