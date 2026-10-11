package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	bolt "go.etcd.io/bbolt"
)

const (
	bucketOutbox    = "event_outbox"
	bucketOutboxDue = "event_outbox_due"
)

func (s *Store) maxPending() int {
	if s.outboxMaxSet {
		return s.outboxMaxPending
	}
	return events.DefaultMaxPending
}

// SetOutboxMaxPending sets the pending cap. Zero disables it.
// Call once at process start, before requests.
func (s *Store) SetOutboxMaxPending(n int) {
	s.outboxMaxSet = true
	s.outboxMaxPending = n
}

func (s *Store) InsertOutbox(_ context.Context, rec events.OutboxRecord) (int, error) {
	var dropped int
	err := s.db.Update(func(tx *bolt.Tx) error {
		n, err := insertOutboxBolt(tx, []events.OutboxRecord{rec}, s.maxPending())
		dropped = n
		return err
	})
	return dropped, err
}

func (s *Store) ListDueOutbox(_ context.Context, now time.Time, limit int) ([]events.OutboxRecord, error) {
	if limit <= 0 {
		limit = 64
	}
	now = now.UTC()
	var out []events.OutboxRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		due := tx.Bucket([]byte(bucketOutboxDue))
		primary := tx.Bucket([]byte(bucketOutbox))
		if due == nil || primary == nil {
			return nil
		}
		c := due.Cursor()
		for k, v := c.First(); k != nil && len(out) < limit; k, v = c.Next() {
			next, ok := parseDueKey(k)
			if !ok {
				continue
			}
			if next.After(now) {
				break
			}
			id := string(v)
			raw := primary.Get([]byte(id))
			if raw == nil {
				continue
			}
			var rec events.OutboxRecord
			if err := json.Unmarshal(raw, &rec); err != nil {
				return err
			}
			if rec.Status != events.StatusPending {
				continue
			}
			out = append(out, rec)
		}
		return nil
	})
	return out, err
}

func (s *Store) SaveOutbox(_ context.Context, rec events.OutboxRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return saveOutboxBolt(tx, rec)
	})
}

func (s *Store) PurgeOutbox(_ context.Context, status string, before time.Time) (int, error) {
	before = before.UTC()
	var n int
	err := s.db.Update(func(tx *bolt.Tx) error {
		primary := tx.Bucket([]byte(bucketOutbox))
		if primary == nil {
			return nil
		}
		var doomed []events.OutboxRecord
		c := primary.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var rec events.OutboxRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.Status != status {
				continue
			}
			ts := retentionTime(rec)
			if ts.IsZero() || !ts.Before(before) {
				continue
			}
			doomed = append(doomed, rec)
		}
		for _, rec := range doomed {
			if err := deleteOutboxBolt(tx, rec); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

func (s *Store) CountOutbox(_ context.Context, status string) (int, error) {
	var n int
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketOutbox))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec events.OutboxRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.Status == status {
				n++
			}
			return nil
		})
	})
	return n, err
}

func (s *Store) GetOutbox(_ context.Context, id string) (events.OutboxRecord, error) {
	var rec events.OutboxRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketOutbox))
		if b == nil {
			return ErrNotFound
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return ErrNotFound
		}
		return json.Unmarshal(raw, &rec)
	})
	return rec, err
}

func insertOutboxBolt(tx *bolt.Tx, evs []events.OutboxRecord, maxPending int) (int, error) {
	if len(evs) == 0 {
		return 0, nil
	}
	primary, err := tx.CreateBucketIfNotExists([]byte(bucketOutbox))
	if err != nil {
		return 0, err
	}
	if _, err := tx.CreateBucketIfNotExists([]byte(bucketOutboxDue)); err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	dropped := 0
	for _, ev := range evs {
		ev = events.NormalizeOutbox(ev, now)
		if maxPending > 0 {
			pending := countPendingBolt(primary)
			for pending >= maxPending {
				if !dropOldestPendingBolt(tx, events.DropReasonCap, now) {
					break
				}
				dropped++
				pending--
			}
		}
		if err := saveOutboxBolt(tx, ev); err != nil {
			return dropped, err
		}
	}
	return dropped, nil
}

func countPendingBolt(primary *bolt.Bucket) int {
	n := 0
	_ = primary.ForEach(func(_, v []byte) error {
		var rec events.OutboxRecord
		if json.Unmarshal(v, &rec) == nil && rec.Status == events.StatusPending {
			n++
		}
		return nil
	})
	return n
}

func dropOldestPendingBolt(tx *bolt.Tx, reason string, now time.Time) bool {
	primary := tx.Bucket([]byte(bucketOutbox))
	if primary == nil {
		return false
	}
	var oldest events.OutboxRecord
	found := false
	_ = primary.ForEach(func(_, v []byte) error {
		var rec events.OutboxRecord
		if json.Unmarshal(v, &rec) != nil || rec.Status != events.StatusPending {
			return nil
		}
		if !found || rec.CreatedAt.Before(oldest.CreatedAt) || (rec.CreatedAt.Equal(oldest.CreatedAt) && rec.ID < oldest.ID) {
			oldest = rec
			found = true
		}
		return nil
	})
	if !found {
		return false
	}
	oldest.Status = events.StatusDropped
	oldest.DropReason = reason
	oldest.UpdatedAt = now
	return saveOutboxBolt(tx, oldest) == nil
}

func saveOutboxBolt(tx *bolt.Tx, rec events.OutboxRecord) error {
	primary, err := tx.CreateBucketIfNotExists([]byte(bucketOutbox))
	if err != nil {
		return err
	}
	due, err := tx.CreateBucketIfNotExists([]byte(bucketOutboxDue))
	if err != nil {
		return err
	}
	if raw := primary.Get([]byte(rec.ID)); raw != nil {
		var old events.OutboxRecord
		if json.Unmarshal(raw, &old) == nil && old.Status == events.StatusPending {
			_ = due.Delete(dueKey(old.NextAttemptAt, old.ID))
		}
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := primary.Put([]byte(rec.ID), data); err != nil {
		return err
	}
	if rec.Status == events.StatusPending {
		return due.Put(dueKey(rec.NextAttemptAt, rec.ID), []byte(rec.ID))
	}
	return nil
}

func deleteOutboxBolt(tx *bolt.Tx, rec events.OutboxRecord) error {
	primary := tx.Bucket([]byte(bucketOutbox))
	due := tx.Bucket([]byte(bucketOutboxDue))
	if due != nil {
		_ = due.Delete(dueKey(rec.NextAttemptAt, rec.ID))
	}
	if primary == nil {
		return nil
	}
	return primary.Delete([]byte(rec.ID))
}

func dueKey(next time.Time, id string) []byte {
	return []byte(fmt.Sprintf("%020d\x00%s", next.UTC().UnixNano(), id))
}

func parseDueKey(k []byte) (time.Time, bool) {
	if len(k) < 21 {
		return time.Time{}, false
	}
	var nano int64
	if _, err := fmt.Sscanf(string(k[:20]), "%d", &nano); err != nil {
		return time.Time{}, false
	}
	return time.Unix(0, nano).UTC(), true
}

func retentionTime(rec events.OutboxRecord) time.Time {
	switch rec.Status {
	case events.StatusDelivered:
		if rec.DeliveredAt != nil {
			return rec.DeliveredAt.UTC()
		}
	case events.StatusDead:
		if rec.DeadAt != nil {
			return rec.DeadAt.UTC()
		}
	case events.StatusDropped:
		return rec.UpdatedAt.UTC()
	}
	return time.Time{}
}

var _ OutboxMutator = (*Store)(nil)
