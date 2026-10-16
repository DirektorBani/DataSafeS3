DROP TRIGGER IF EXISTS ha_leader_attempt_bi ON ha_leader_lock;
DROP FUNCTION IF EXISTS ha_leader_attempt_record();
DROP TABLE IF EXISTS ha_leader_attempts;
