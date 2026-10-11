package events

import (
	"context"
	"errors"
	"strings"
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
	failKey string
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
	if m.failFor > 0 && (m.failKey == "" || e.Key == m.failKey) {
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

func (m *memSink) snapshot() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Event, len(m.events))
	copy(out, m.events)
	return out
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

type staticErrSink struct {
	name string
	err  error
}

func (s *staticErrSink) Name() string { return s.name }
func (s *staticErrSink) Publish(context.Context, Event) error {
	return s.err
}
func (s *staticErrSink) Close() error { return nil }

func TestDispatcherTerminalErrorIsDeadImmediately(t *testing.T) {
	store := newMemOutbox()
	sink := &staticErrSink{name: "webhook", err: &TerminalError{Err: errors.New("webhook wh: 400 Bad Request")}}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 20, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d.purgeEvery = time.Hour
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	rec := NormalizeOutbox(OutboxRecord{
		EventType: "ObjectCreated",
		Attempts: map[string]SinkAttempt{
			"webhook": {Status: StatusPending, NextAttemptAt: now},
		},
	}, now)
	if _, err := store.InsertOutbox(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if err := d.ProcessOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDead {
		t.Fatalf("status %s %+v", got.Status, got.Attempts)
	}
	att := got.Attempts["webhook"]
	if att.Attempts != 1 || att.Status != StatusDead || !strings.Contains(att.LastError, "400") {
		t.Fatalf("attempt %+v", att)
	}
}

func TestDispatcherRetryAfterIsCapped(t *testing.T) {
	store := newMemOutbox()
	sink := &staticErrSink{name: "webhook", err: &RetryAfterError{Err: errors.New("429 Too Many Requests"), After: 10 * time.Minute}}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 20, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d.purgeEvery = time.Hour
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	rec := NormalizeOutbox(OutboxRecord{
		EventType: "ObjectCreated",
		Attempts: map[string]SinkAttempt{
			"webhook": {Status: StatusPending, NextAttemptAt: now},
		},
	}, now)
	if _, err := store.InsertOutbox(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if err := d.ProcessOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusPending {
		t.Fatalf("status %s", got.Status)
	}
	att := got.Attempts["webhook"]
	if att.Attempts != 1 || !att.NextAttemptAt.Equal(now.Add(maxBackoff)) {
		t.Fatalf("next %s want %s (%+v)", att.NextAttemptAt, now.Add(maxBackoff), att)
	}
}

func orderRow(id, eventType, tenant, bucket, key string, at time.Time) OutboxRecord {
	return NormalizeOutbox(OutboxRecord{
		ID:        id,
		EventType: eventType,
		TenantID:  tenant,
		Bucket:    bucket,
		Key:       key,
		CreatedAt: at,
		Attempts: map[string]SinkAttempt{
			"kafka": {Status: StatusPending, NextAttemptAt: at},
		},
	}, at)
}

func TestDispatcherDeliversSameKeyInOrderAfterFailure(t *testing.T) {
	store := newMemOutbox()
	sink := &memSink{name: "kafka", failFor: 1, failKey: "obj"}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 5, Retention: 24 * time.Hour, DeadRetention: 24 * time.Hour}, nil, Observer{})
	d.purgeEvery = 24 * time.Hour
	t0 := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	// The first tick purges when lastPurge is zero. Delivered rows must still
	// be readable after the loop, so this test does not purge inside the window.
	d.lastPurge = t0
	created := orderRow("01CREATE000000000000000000", "ObjectCreated", "ten", "b", "obj", t0)
	deleted := orderRow("01DELETE000000000000000000", "ObjectDeleted", "ten", "b", "obj", t0.Add(time.Second))
	other := orderRow("01OTHER0000000000000000000", "ObjectCreated", "ten", "b", "other", t0.Add(2*time.Second))
	// Insert the later row first. ListDue walks a map, so delivery order is not insert order.
	if _, err := store.InsertOutbox(context.Background(), deleted); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertOutbox(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertOutbox(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if err := d.ProcessOnce(ctx, t0.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	got := sink.snapshot()
	if len(got) != 3 {
		t.Fatalf("delivered %d %+v", len(got), got)
	}
	if got[0].Key == "obj" && got[0].Type != "ObjectCreated" {
		t.Fatalf("first obj event %s, want ObjectCreated", got[0].Type)
	}
	var obj []string
	var sawOther bool
	for _, e := range got {
		if e.Key == "other" {
			sawOther = true
			continue
		}
		obj = append(obj, e.Type)
	}
	if !sawOther {
		t.Fatal("other key was blocked by the stuck key")
	}
	if len(obj) != 2 || obj[0] != "ObjectCreated" || obj[1] != "ObjectDeleted" {
		t.Fatalf("obj order %v", obj)
	}
	// The other key is delivered while obj's create is still failing or already recovered.
	// It must not be held until obj is done: it appears, and obj stays in order.
	del, err := store.GetOutbox(ctx, deleted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if del.Status != StatusDelivered {
		t.Fatalf("delete status %s", del.Status)
	}
}

func TestDispatcherLaterRowProceedsWhenEarlierIsDead(t *testing.T) {
	store := newMemOutbox()
	sink := &memSink{name: "kafka", failFor: 1, failKey: "obj"}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 1, Retention: 24 * time.Hour, DeadRetention: 24 * time.Hour}, nil, Observer{})
	d.purgeEvery = 24 * time.Hour
	t0 := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	d.lastPurge = t0
	created := orderRow("01CREATE000000000000000000", "ObjectCreated", "ten", "b", "obj", t0)
	deleted := orderRow("01DELETE000000000000000000", "ObjectDeleted", "ten", "b", "obj", t0.Add(time.Second))
	if _, err := store.InsertOutbox(context.Background(), deleted); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertOutbox(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := d.ProcessOnce(ctx, t0.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	cr, err := store.GetOutbox(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cr.Attempts["kafka"].Status != StatusDead {
		t.Fatalf("create sink %+v", cr.Attempts["kafka"])
	}
	got := sink.snapshot()
	if len(got) != 1 || got[0].Type != "ObjectDeleted" || got[0].ID != deleted.ID {
		t.Fatalf("after dead earlier row, got %+v", got)
	}
}

func TestDispatcherEmptyKeyIsNotOrdered(t *testing.T) {
	store := newMemOutbox()
	sink := &memSink{name: "kafka", failFor: 1}
	d := NewDispatcher(store, []EventSink{sink}, Config{MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{})
	d.purgeEvery = time.Hour
	t0 := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	first := orderRow("01USERA0000000000000000000", "UserCreated", "ten", "", "", t0)
	second := orderRow("01USERB0000000000000000000", "UserCreated", "ten", "", "", t0.Add(time.Second))
	_, _ = store.InsertOutbox(context.Background(), second)
	_, _ = store.InsertOutbox(context.Background(), first)
	if err := d.ProcessOnce(context.Background(), t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if sink.count() != 1 {
		t.Fatalf("empty-key later row should publish while the earlier row fails, got %d", sink.count())
	}
}
