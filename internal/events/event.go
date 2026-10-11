// Package events is the notification fan-out for DataSafeS3.
//
// One EventSink (Name, Publish, Close) delivers records from the metadata
// outbox. Webhook and NATS adapt to it today. A Kafka sink (E02) registers
// on the Dispatcher when STORAGE_KAFKA_BROKERS is set; this package does not
// open a broker connection by itself.
package events

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"time"
)

const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusDead      = "dead"
	StatusDropped   = "dropped"

	// DropReasonCap is stored on rows the pending cap evicts.
	DropReasonCap = "pending cap exceeded"
)

// Event is the payload a sink publishes. Payload is the same map historically
// passed to webhooks and NATS (the "data" / "payload" object).
type Event struct {
	ID        string
	Type      string
	Bucket    string
	Key       string
	VersionID string
	TenantID  string
	CreatedAt time.Time
	Payload   map[string]any
}

// EventSink delivers one event to an external system.
// Publish must return a non-nil error when the attempt failed and should be retried.
// A disabled sink (for example NATS with an empty URL) returns nil and must not dial.
type EventSink interface {
	Name() string
	Publish(ctx context.Context, e Event) error
	Close() error
}

// SinkAttempt is per-sink delivery state stored on the outbox row.
type SinkAttempt struct {
	Attempts      int       `json:"attempts"`
	Status        string    `json:"status"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	LastError     string    `json:"last_error,omitempty"`
}

// OutboxRecord is one durable notification.
// Attempts is keyed by sink name snapshotted when the row is written.
// Later sinks do not inherit rows that were already queued.
type OutboxRecord struct {
	ID            string                 `json:"id"`
	CreatedAt     time.Time              `json:"created_at"`
	EventType     string                 `json:"event_type"`
	Bucket        string                 `json:"bucket,omitempty"`
	Key           string                 `json:"key,omitempty"`
	VersionID     string                 `json:"version_id,omitempty"`
	TenantID      string                 `json:"tenant_id,omitempty"`
	Payload       json.RawMessage        `json:"payload"`
	Status        string                 `json:"status"`
	Attempts      map[string]SinkAttempt `json:"attempts"`
	NextAttemptAt time.Time              `json:"next_attempt_at"`
	DropReason    string                 `json:"drop_reason,omitempty"`
	DeliveredAt   *time.Time             `json:"delivered_at,omitempty"`
	DeadAt        *time.Time             `json:"dead_at,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// Event decodes the stored payload into the sink view.
func (rec OutboxRecord) Event() Event {
	var payload map[string]any
	if len(rec.Payload) > 0 {
		_ = json.Unmarshal(rec.Payload, &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return Event{
		ID:        rec.ID,
		Type:      rec.EventType,
		Bucket:    rec.Bucket,
		Key:       rec.Key,
		VersionID: rec.VersionID,
		TenantID:  rec.TenantID,
		CreatedAt: rec.CreatedAt,
		Payload:   payload,
	}
}

// NormalizeOutbox fills id, timestamps, and status when the caller left them empty.
// It does not overwrite a status or id that is already set.
func NormalizeOutbox(rec OutboxRecord, now time.Time) OutboxRecord {
	now = now.UTC()
	if rec.ID == "" {
		rec.ID = NewULID(now)
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.CreatedAt = rec.CreatedAt.UTC()
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = now
	}
	rec.UpdatedAt = rec.UpdatedAt.UTC()
	if len(rec.Payload) == 0 {
		rec.Payload = []byte(`{}`)
	}
	if rec.Attempts == nil {
		rec.Attempts = map[string]SinkAttempt{}
	}
	if rec.NextAttemptAt.IsZero() {
		rec.NextAttemptAt = now
	}
	rec.NextAttemptAt = rec.NextAttemptAt.UTC()
	if rec.Status == "" {
		if len(rec.Attempts) == 0 {
			rec.Status = StatusDelivered
			t := now
			rec.DeliveredAt = &t
		} else {
			rec.Status = StatusPending
		}
	}
	return rec
}

// AggregateStatus sets the row status from per-sink attempts.
// pending if any sink is still pending; dead if none are pending and any is dead;
// delivered when every sink succeeded. Dropped rows are left unchanged.
func AggregateStatus(rec *OutboxRecord, now time.Time) {
	if rec == nil || rec.Status == StatusDropped {
		return
	}
	now = now.UTC()
	rec.UpdatedAt = now
	if len(rec.Attempts) == 0 {
		rec.Status = StatusDelivered
		if rec.DeliveredAt == nil {
			t := now
			rec.DeliveredAt = &t
		}
		return
	}
	anyPending := false
	anyDead := false
	var next time.Time
	for _, a := range rec.Attempts {
		switch a.Status {
		case StatusDead:
			anyDead = true
		case StatusDelivered:
			// success
		default:
			anyPending = true
			if next.IsZero() || a.NextAttemptAt.Before(next) {
				next = a.NextAttemptAt
			}
		}
	}
	switch {
	case anyPending:
		rec.Status = StatusPending
		if !next.IsZero() {
			rec.NextAttemptAt = next.UTC()
		}
	case anyDead:
		rec.Status = StatusDead
		if rec.DeadAt == nil {
			t := now
			rec.DeadAt = &t
		}
	default:
		rec.Status = StatusDelivered
		if rec.DeliveredAt == nil {
			t := now
			rec.DeliveredAt = &t
		}
	}
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewULID returns a 26-character ULID. The time prefix sorts lexicographically.
func NewULID(now time.Time) string {
	var id [16]byte
	ms := uint64(now.UTC().UnixMilli())
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	_, _ = rand.Read(id[6:])
	return encodeULID(id)
}

func encodeULID(id [16]byte) string {
	dst := make([]byte, 26)
	dst[0] = crockford[(id[0]&224)>>5]
	dst[1] = crockford[id[0]&31]
	dst[2] = crockford[(id[1]&248)>>3]
	dst[3] = crockford[((id[1]&7)<<2)|((id[2]&192)>>6)]
	dst[4] = crockford[(id[2]&62)>>1]
	dst[5] = crockford[((id[2]&1)<<4)|((id[3]&240)>>4)]
	dst[6] = crockford[((id[3]&15)<<1)|((id[4]&128)>>7)]
	dst[7] = crockford[(id[4]&124)>>2]
	dst[8] = crockford[((id[4]&3)<<3)|((id[5]&224)>>5)]
	dst[9] = crockford[id[5]&31]
	dst[10] = crockford[(id[6]&248)>>3]
	dst[11] = crockford[((id[6]&7)<<2)|((id[7]&192)>>6)]
	dst[12] = crockford[(id[7]&62)>>1]
	dst[13] = crockford[((id[7]&1)<<4)|((id[8]&240)>>4)]
	dst[14] = crockford[((id[8]&15)<<1)|((id[9]&128)>>7)]
	dst[15] = crockford[(id[9]&124)>>2]
	dst[16] = crockford[((id[9]&3)<<3)|((id[10]&224)>>5)]
	dst[17] = crockford[id[10]&31]
	dst[18] = crockford[(id[11]&248)>>3]
	dst[19] = crockford[((id[11]&7)<<2)|((id[12]&192)>>6)]
	dst[20] = crockford[(id[12]&62)>>1]
	dst[21] = crockford[((id[12]&1)<<4)|((id[13]&240)>>4)]
	dst[22] = crockford[((id[13]&15)<<1)|((id[14]&128)>>7)]
	dst[23] = crockford[(id[14]&124)>>2]
	dst[24] = crockford[((id[14]&3)<<3)|((id[15]&224)>>5)]
	dst[25] = crockford[id[15]&31]
	return string(dst)
}
