-- Teams and bucket visibility (owner_id, team_id)
CREATE TABLE IF NOT EXISTS teams (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_teams (
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id     TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, team_id)
);
CREATE INDEX IF NOT EXISTS idx_user_teams_team ON user_teams(team_id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS team_id TEXT REFERENCES teams(id);
CREATE INDEX IF NOT EXISTS idx_users_team ON users(team_id);

ALTER TABLE buckets ADD COLUMN IF NOT EXISTS owner_id TEXT;
ALTER TABLE buckets ADD COLUMN IF NOT EXISTS team_id TEXT REFERENCES teams(id);
CREATE INDEX IF NOT EXISTS idx_buckets_owner_id ON buckets(owner_id);
CREATE INDEX IF NOT EXISTS idx_buckets_team_id ON buckets(team_id);

UPDATE buckets b
SET owner_id = u.id
FROM users u
WHERE b.owner_id IS NULL AND b.owner = u.username;
