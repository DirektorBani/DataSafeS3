package events

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errNotFound = errors.New("not found")

type memOutbox struct {
	mu   sync.Mutex
	rows map[string]OutboxRecord
}

func newMemOutbox() *memOutbox {
	return &memOutbox{rows: map[string]OutboxRecord{}}
}

func (m *memOutbox) InsertOutbox(_ context.Context, rec OutboxRecord) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec = NormalizeOutbox(rec, time.Now().UTC())
	m.rows[rec.ID] = rec
	return 0, nil
}

func (m *memOutbox) ListDueOutbox(_ context.Context, now time.Time, limit int) ([]OutboxRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 {
		limit = 64
	}
	var out []OutboxRecord
	for _, rec := range m.rows {
		if rec.Status == StatusPending && !rec.NextAttemptAt.After(now) {
			out = append(out, rec)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (m *memOutbox) SaveOutbox(_ context.Context, rec OutboxRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[rec.ID] = rec
	return nil
}

func (m *memOutbox) PurgeOutbox(_ context.Context, status string, before time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for id, rec := range m.rows {
		if rec.Status != status {
			continue
		}
		var ts time.Time
		switch status {
		case StatusDelivered:
			if rec.DeliveredAt != nil {
				ts = *rec.DeliveredAt
			}
		case StatusDead:
			if rec.DeadAt != nil {
				ts = *rec.DeadAt
			}
		case StatusDropped:
			ts = rec.UpdatedAt
		}
		if !ts.IsZero() && ts.Before(before) {
			delete(m.rows, id)
			n++
		}
	}
	return n, nil
}

func (m *memOutbox) CountOutbox(_ context.Context, status string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, rec := range m.rows {
		if rec.Status == status {
			n++
		}
	}
	return n, nil
}

func (m *memOutbox) GetOutbox(_ context.Context, id string) (OutboxRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.rows[id]
	if !ok {
		return OutboxRecord{}, errNotFound
	}
	return rec, nil
}
