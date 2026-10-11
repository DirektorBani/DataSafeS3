CREATE TABLE IF NOT EXISTS event_outbox (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    event_type TEXT NOT NULL,
    bucket TEXT NOT NULL DEFAULT '',
    key TEXT NOT NULL DEFAULT '',
    version_id TEXT NOT NULL DEFAULT '',
    tenant_id TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    attempts JSONB NOT NULL DEFAULT '{}'::jsonb,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    drop_reason TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ,
    dead_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_event_outbox_due
    ON event_outbox (status, next_attempt_at);

CREATE INDEX IF NOT EXISTS idx_event_outbox_pending_created
    ON event_outbox (created_at, id)
    WHERE status = 'pending';
