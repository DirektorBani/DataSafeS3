// Package events defines the EventSink fan-out contract for S3/admin notifications.
// NATS is implemented today in internal/api/events_nats.go; Kafka (A3) should implement EventSink.
// See docs/architecture/adr/0002-event-sink-interface.md.
package events

import "context"

// NotificationEvent is a minimal bus payload (expand in A3 without breaking Name/Publish/Close).
type NotificationEvent struct {
	Name   string
	Bucket string
	Key    string
	Size   int64
}

// EventSink fans out notifications to an external bus.
type EventSink interface {
	Name() string
	Publish(ctx context.Context, e NotificationEvent) error
	Close() error
}
