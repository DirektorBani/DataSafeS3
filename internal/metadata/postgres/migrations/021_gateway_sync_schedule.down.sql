ALTER TABLE replication_rules
    DROP COLUMN IF EXISTS sync_schedule,
    DROP COLUMN IF EXISTS last_sync_at,
    DROP COLUMN IF EXISTS last_sync_status,
    DROP COLUMN IF EXISTS last_sync_error,
    DROP COLUMN IF EXISTS last_sync_objects;
