DROP TABLE IF EXISTS mfa_stepup_failures;
DROP TABLE IF EXISTS mfa_denied_events;
ALTER TABLE users DROP COLUMN IF EXISTS mfa_reset_required;
ALTER TABLE users DROP COLUMN IF EXISTS idp_mfa;
