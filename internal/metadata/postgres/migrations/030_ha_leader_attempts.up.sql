-- 1.5.0 followers retry the same INSERT as the leader. While the lease is held
-- the ON CONFLICT WHERE does not match, so ha_leader_lock stays with the
-- leader and the follower id is not stored. A BEFORE INSERT trigger still
-- runs for that statement, including when the update is skipped.
CREATE TABLE IF NOT EXISTS ha_leader_attempts (
    holder_id TEXT PRIMARY KEY,
    attempted_at TIMESTAMPTZ NOT NULL
);

CREATE OR REPLACE FUNCTION ha_leader_attempt_record() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO ha_leader_attempts (holder_id, attempted_at)
  VALUES (NEW.holder_id, clock_timestamp())
  ON CONFLICT (holder_id) DO UPDATE SET attempted_at = EXCLUDED.attempted_at;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS ha_leader_attempt_bi ON ha_leader_lock;
CREATE TRIGGER ha_leader_attempt_bi
  BEFORE INSERT ON ha_leader_lock
  FOR EACH ROW
  EXECUTE FUNCTION ha_leader_attempt_record();
