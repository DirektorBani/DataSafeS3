package events

import (
	"context"
	"time"
)

// OutboxStore persists notification rows in the metadata database.
// Bolt and Postgres both implement it. The dispatcher depends only on this
// interface so a later Kafka sink does not need a new store.
type OutboxStore interface {
	// InsertOutbox writes rec. When the pending cap is set and full, the oldest
	// pending rows are marked dropped and the new row is still inserted.
	// The returned count is how many rows were dropped. An error is a store
	// failure, not the cap.
	InsertOutbox(ctx context.Context, rec OutboxRecord) (dropped int, err error)
	ListDueOutbox(ctx context.Context, now time.Time, limit int) ([]OutboxRecord, error)
	SaveOutbox(ctx context.Context, rec OutboxRecord) error
	// PurgeOutbox deletes rows of status whose retention timestamp is before cutoff.
	// delivered uses delivered_at, dead uses dead_at, dropped uses updated_at.
	PurgeOutbox(ctx context.Context, status string, before time.Time) (int, error)
	CountOutbox(ctx context.Context, status string) (int, error)
	GetOutbox(ctx context.Context, id string) (OutboxRecord, error)
}
