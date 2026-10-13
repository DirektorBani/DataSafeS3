ALTER TABLE users ADD COLUMN IF NOT EXISTS idp_mfa BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_reset_required BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS mfa_denied_events (
    user_id TEXT NOT NULL,
    action TEXT NOT NULL,
    last_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, action)
);

CREATE TABLE IF NOT EXISTS mfa_stepup_failures (
    user_id TEXT PRIMARY KEY,
    failures INT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    locked_until TIMESTAMPTZ
);
