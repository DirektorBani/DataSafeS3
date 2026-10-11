package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memSink struct {
	name    string
	mu      sync.Mutex
	events  []Event
	closed  bool
	failFor int
	calls   int
}

func (m *memSink) Name() string { return m.name }

func (m *memSink) Publish(ctx context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.closed {
		return errors.New("closed")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if m.failFor > 0 {
		m.failFor--
		return errors.New("unavailable")
	}
	m.events = append(m.events, e)
	return nil
}

func (m *memSink) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *memSink) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func TestEventSinkContract(t *testing.T) {
	var s EventSink = &memSink{name: "mem"}
	if s.Name() != "mem" {
		t.Fatalf("Name=%q", s.Name())
	}
	e := Event{Type: "ObjectCreated", Bucket: "b", Key: "k"}
	if err := s.Publish(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(context.Background(), e); err == nil {
		t.Fatal("expected error after Close")
	}
}

func TestEventSinkRespectsCanceledContext(t *testing.T) {
	s := &memSink{name: "mem"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Publish(ctx, Event{Type: "x"}); err == nil {
		t.Fatal("expected ctx error")
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("STORAGE_EVENTS_RETENTION", "")
	t.Setenv("STORAGE_EVENTS_DEAD_RETENTION", "")
	t.Setenv("STORAGE_EVENTS_MAX_PENDING", "")
	t.Setenv("STORAGE_EVENTS_MAX_ATTEMPTS", "")
	cfg := ConfigFromEnv()
	if cfg.Retention != DefaultRetention || cfg.DeadRetention != DefaultDeadRetention {
		t.Fatalf("retention %+v", cfg)
	}
	if cfg.MaxPending != DefaultMaxPending || cfg.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("limits %+v", cfg)
	}
}

func TestConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("STORAGE_EVENTS_RETENTION", "1h")
	t.Setenv("STORAGE_EVENTS_DEAD_RETENTION", "48h")
	t.Setenv("STORAGE_EVENTS_MAX_PENDING", "3")
	t.Setenv("STORAGE_EVENTS_MAX_ATTEMPTS", "4")
	cfg := ConfigFromEnv()
	if cfg.Retention != time.Hour || cfg.DeadRetention != 48*time.Hour || cfg.MaxPending != 3 || cfg.MaxAttempts != 4 {
		t.Fatalf("%+v", cfg)
	}
	t.Setenv("STORAGE_EVENTS_RETENTION", "nope")
	t.Setenv("STORAGE_EVENTS_MAX_PENDING", "-1")
	cfg = ConfigFromEnv()
	if cfg.Retention != DefaultRetention || cfg.MaxPending != DefaultMaxPending {
		t.Fatalf("invalid values should keep defaults: %+v", cfg)
	}
}

func TestULIDSortsByTime(t *testing.T) {
	a := NewULID(time.UnixMilli(1_700_000_000_000))
	b := NewULID(time.UnixMilli(1_700_000_000_100))
	if a >= b {
		t.Fatalf("ulid order %s >= %s", a, b)
	}
	if len(a) != 26 {
		t.Fatalf("len %d", len(a))
	}
}

func TestBackoffGrowsAndCaps(t *testing.T) {
	var prev time.Duration
	for attempt := 1; attempt <= 8; attempt++ {
		d := BackoffDuration(attempt)
		if d > maxBackoff {
			t.Fatalf("attempt %d duration %s over cap", attempt, d)
		}
		if attempt > 1 && d < prev/2 {
			t.Fatalf("attempt %d = %s shrank too far from %s", attempt, d, prev)
		}
		prev = d
	}
	if BackoffDuration(30) > maxBackoff {
		t.Fatal("uncapped")
	}
}

func TestDispatcherFailingSinkBecomesDead(t *testing.T) {
	store := newMemOutbox()
	sink := &memSink{name: "webhook", failFor: 100}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 3, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d.purgeEvery = time.Hour
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	rec := NormalizeOutbox(OutboxRecord{
		EventType: "ObjectCreated",
		Bucket:    "b",
		Key:       "k",
		Attempts: map[string]SinkAttempt{
			"webhook": {Status: StatusPending, NextAttemptAt: now},
		},
	}, now)
	if _, err := store.InsertOutbox(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := d.ProcessOnce(context.Background(), now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDead {
		t.Fatalf("status %s attempts %+v", got.Status, got.Attempts)
	}
	if got.Attempts["webhook"].Attempts != 3 {
		t.Fatalf("attempts %+v", got.Attempts["webhook"])
	}
	if sink.count() != 0 {
		t.Fatal("sink should not have accepted the event")
	}
}

func TestDispatcherRetriesThenDelivers(t *testing.T) {
	store := newMemOutbox()
	sink := &memSink{name: "nats", failFor: 2}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d.purgeEvery = time.Hour
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	rec := NormalizeOutbox(OutboxRecord{
		EventType: "ObjectCreated",
		Attempts: map[string]SinkAttempt{
			"nats": {Status: StatusPending, NextAttemptAt: now},
		},
	}, now)
	_, _ = store.InsertOutbox(context.Background(), rec)
	for i := 0; i < 3; i++ {
		if err := d.ProcessOnce(context.Background(), now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDelivered {
		t.Fatalf("status %s %+v", got.Status, got.Attempts)
	}
	if sink.count() != 1 {
		t.Fatalf("delivered %d", sink.count())
	}
}

func TestDispatcherSkipsNonLeader(t *testing.T) {
	store := newMemOutbox()
	sink := &memSink{name: "webhook"}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 3, Retention: time.Hour, DeadRetention: time.Hour}, func(context.Context) bool { return false }, Observer{})
	now := time.Now().UTC()
	rec := NormalizeOutbox(OutboxRecord{
		EventType: "ObjectCreated",
		Attempts: map[string]SinkAttempt{
			"webhook": {Status: StatusPending, NextAttemptAt: now},
		},
	}, now)
	_, _ = store.InsertOutbox(context.Background(), rec)
	if err := d.ProcessOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetOutbox(context.Background(), rec.ID)
	if got.Status != StatusPending {
		t.Fatalf("follower delivered: %s", got.Status)
	}
	if sink.count() != 0 {
		t.Fatal("follower sink was called")
	}
}

func TestDispatcherRestartMidDelivery(t *testing.T) {
	store := newMemOutbox()
	now := time.Now().UTC()
	rec := NormalizeOutbox(OutboxRecord{
		EventType: "ObjectCreated",
		Bucket:    "b",
		Key:       "k",
		Attempts: map[string]SinkAttempt{
			"webhook": {Status: StatusPending, NextAttemptAt: now},
		},
	}, now)
	_, _ = store.InsertOutbox(context.Background(), rec)
	first := &memSink{name: "webhook", failFor: 1}
	d1 := NewDispatcher(store, []EventSink{first}, Config{MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d1.purgeEvery = time.Hour
	if err := d1.ProcessOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	_ = d1.Close()
	second := &memSink{name: "webhook"}
	d2 := NewDispatcher(store, []EventSink{second}, Config{MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d2.purgeEvery = time.Hour
	if err := d2.ProcessOnce(context.Background(), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDelivered || second.count() != 1 {
		t.Fatalf("status %s delivered %d", got.Status, second.count())
	}
}

func TestRetentionPurge(t *testing.T) {
	store := newMemOutbox()
	old := time.Now().UTC().Add(-48 * time.Hour)
	rec := NormalizeOutbox(OutboxRecord{
		EventType:   "ObjectCreated",
		Status:      StatusDelivered,
		DeliveredAt: &old,
		UpdatedAt:   old,
	}, old)
	_, _ = store.InsertOutbox(context.Background(), rec)
	d := NewDispatcher(store, nil, Config{Retention: 24 * time.Hour, DeadRetention: 168 * time.Hour, MaxAttempts: 2}, nil, Observer{})
	d.purgeEvery = 0
	if err := d.ProcessOnce(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOutbox(context.Background(), rec.ID); err == nil {
		t.Fatal("delivered row should have been purged")
	}
}
