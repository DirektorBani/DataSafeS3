package metadata

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
)

func TestBoltOutboxRestartMidDelivery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")
	s, err := OpenBolt(path)
	if err != nil {
		t.Fatal(err)
	}
	s.SetOutboxMaxPending(100)
	now := time.Now().UTC().Add(-time.Minute)
	rec := events.NormalizeOutbox(events.OutboxRecord{
		EventType: "ObjectCreated",
		Bucket:    "b",
		Key:       "k",
		CreatedAt: now,
		Attempts: map[string]events.SinkAttempt{
			"webhook": {Status: events.StatusPending, NextAttemptAt: now},
		},
	}, now)
	if _, err := s.InsertOutbox(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := OpenBolt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	sink := &recordingSink{name: "webhook"}
	d := events.NewDispatcher(s2, []events.EventSink{sink}, events.Config{
		MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	d.ProcessOnce(context.Background(), time.Now().UTC())
	if sink.n != 1 {
		t.Fatalf("delivered %d", sink.n)
	}
	got, err := s2.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusDelivered {
		t.Fatalf("status %s", got.Status)
	}
}

func TestBoltOutboxCapDoesNotFailObjectWrite(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenBolt(filepath.Join(dir, "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetOutboxMaxPending(2)
	now := time.Now().UTC()
	for i, id := range []string{"01ARZ3NDEKTSV4RRFFQ69G5FA1", "01ARZ3NDEKTSV4RRFFQ69G5FA2", "01ARZ3NDEKTSV4RRFFQ69G5FA3"} {
		rec := events.OutboxRecord{
			ID:        id,
			CreatedAt: now.Add(time.Duration(i) * time.Second),
			EventType: "ObjectCreated",
			Bucket:    "b",
			Key:       id,
			Status:    events.StatusPending,
			Attempts: map[string]events.SinkAttempt{
				"webhook": {Status: events.StatusPending, NextAttemptAt: now},
			},
			NextAttemptAt: now,
		}
		obj := ObjectRecord{Bucket: "b", Key: id, Size: 1, LastModified: now}
		dropped, err := s.PutObjectWithOutbox(obj, []events.OutboxRecord{rec})
		if err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
		if i == 2 && dropped != 1 {
			t.Fatalf("dropped %d", dropped)
		}
		if _, err := s.GetObject("b", id); err != nil {
			t.Fatalf("object %s missing: %v", id, err)
		}
	}
	pending, err := s.CountOutbox(context.Background(), events.StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 2 {
		t.Fatalf("pending %d", pending)
	}
	oldest, err := s.GetOutbox(context.Background(), "01ARZ3NDEKTSV4RRFFQ69G5FA1")
	if err != nil {
		t.Fatal(err)
	}
	if oldest.Status != events.StatusDropped || oldest.DropReason != events.DropReasonCap {
		t.Fatalf("oldest %+v", oldest)
	}
}

func TestBoltFailingSinkPersistsDead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")
	s, err := OpenBolt(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rec := events.NormalizeOutbox(events.OutboxRecord{
		EventType: "ObjectCreated",
		Attempts: map[string]events.SinkAttempt{
			"webhook": {Status: events.StatusPending, NextAttemptAt: now},
		},
	}, now)
	if _, err := s.InsertOutbox(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	sink := &failSink{name: "webhook"}
	d := events.NewDispatcher(s, []events.EventSink{sink}, events.Config{
		MaxAttempts: 2, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	if err := d.ProcessOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenBolt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempts["webhook"].Attempts != 1 || got.Status != events.StatusPending {
		t.Fatalf("after restart %+v", got.Attempts["webhook"])
	}
	d2 := events.NewDispatcher(s2, []events.EventSink{&failSink{name: "webhook"}}, events.Config{
		MaxAttempts: 2, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	if err := d2.ProcessOnce(context.Background(), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err = s2.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusDead {
		t.Fatalf("status %s", got.Status)
	}
}

type recordingSink struct {
	name string
	n    int
}

func (r *recordingSink) Name() string { return r.name }
func (r *recordingSink) Publish(context.Context, events.Event) error {
	r.n++
	return nil
}
func (r *recordingSink) Close() error { return nil }

type failSink struct{ name string }

func (f *failSink) Name() string { return f.name }
func (f *failSink) Publish(context.Context, events.Event) error {
	return errFail
}
func (f *failSink) Close() error { return nil }

func TestBoltEarlierPendingSameKey(t *testing.T) {
	s, err := OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetOutboxMaxPending(0)
	t0 := time.Now().UTC().Add(-time.Minute)
	created := events.NormalizeOutbox(events.OutboxRecord{
		ID: "01CREATE000000000000000000", EventType: "ObjectCreated",
		TenantID: "ten", Bucket: "b", Key: "obj", CreatedAt: t0,
		Attempts: map[string]events.SinkAttempt{"kafka": {Status: events.StatusPending, NextAttemptAt: t0}},
	}, t0)
	deleted := events.NormalizeOutbox(events.OutboxRecord{
		ID: "01DELETE000000000000000000", EventType: "ObjectDeleted",
		TenantID: "ten", Bucket: "b", Key: "obj", CreatedAt: t0.Add(time.Second),
		Attempts: map[string]events.SinkAttempt{"kafka": {Status: events.StatusPending, NextAttemptAt: t0}},
	}, t0.Add(time.Second))
	if _, err := s.InsertOutbox(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertOutbox(context.Background(), deleted); err != nil {
		t.Fatal(err)
	}
	blocked, err := s.EarlierPending(context.Background(), "ten", "b", "obj", "kafka", deleted.ID, deleted.CreatedAt)
	if err != nil || !blocked {
		t.Fatalf("blocked %v err %v", blocked, err)
	}
	other, err := s.EarlierPending(context.Background(), "ten", "b", "other", "kafka", deleted.ID, deleted.CreatedAt)
	if err != nil || other {
		t.Fatalf("other key blocked %v err %v", other, err)
	}
	created.Attempts["kafka"] = events.SinkAttempt{Status: events.StatusDead, Attempts: 1}
	events.AggregateStatus(&created, time.Now().UTC())
	if err := s.SaveOutbox(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	blocked, err = s.EarlierPending(context.Background(), "ten", "b", "obj", "kafka", deleted.ID, deleted.CreatedAt)
	if err != nil || blocked {
		t.Fatalf("after dead earlier, blocked %v err %v", blocked, err)
	}
}

var errFail = errString("sink down")

type errString string

func (e errString) Error() string { return string(e) }
