-- First rejected public request claims one of these timestamps.
-- Later rejects of the same link do not emit ShareExpired or ShareLimitReached again.

ALTER TABLE shared_links ADD COLUMN IF NOT EXISTS limit_event_at TIMESTAMPTZ;
ALTER TABLE shared_links ADD COLUMN IF NOT EXISTS expired_event_at TIMESTAMPTZ;
