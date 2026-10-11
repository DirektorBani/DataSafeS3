package postgres

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/ha"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func openOutboxPostgres(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres outbox test")
	}
	s, err := Open(dsn, "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPostgresOutboxCapAndRestart(t *testing.T) {
	s := openOutboxPostgres(t)
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `DELETE FROM event_outbox`); err != nil {
		t.Fatal(err)
	}
	s.SetOutboxMaxPending(2)
	now := time.Now().UTC()
	var ids []string
	for i := 0; i < 3; i++ {
		rec := events.NormalizeOutbox(events.OutboxRecord{
			CreatedAt: now.Add(time.Duration(i) * time.Second),
			EventType: "ObjectCreated",
			Bucket:    "pg-outbox",
			Key:       time.Now().Format("150405.000") + string(rune('a'+i)),
			Attempts: map[string]events.SinkAttempt{
				"webhook": {Status: events.StatusPending, NextAttemptAt: now},
			},
		}, now.Add(time.Duration(i)*time.Second))
		dropped, err := s.InsertOutbox(ctx, rec)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.ID)
		if i == 2 && dropped != 1 {
			t.Fatalf("dropped %d", dropped)
		}
		t.Cleanup(func() {
			_, _ = s.pool.Exec(context.Background(), `DELETE FROM event_outbox WHERE id=$1`, rec.ID)
		})
	}
	pending, err := s.CountOutbox(ctx, events.StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if pending != 2 {
		t.Fatalf("pending %d", pending)
	}
	oldest, err := s.GetOutbox(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if oldest.Status != events.StatusDropped {
		t.Fatalf("oldest status %s", oldest.Status)
	}
	newest, err := s.GetOutbox(ctx, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if newest.Status != events.StatusPending {
		t.Fatalf("newest status %s", newest.Status)
	}

	sink := &pgRecordingSink{name: "webhook"}
	d := events.NewDispatcher(s, []events.EventSink{sink}, events.Config{
		MaxAttempts: 4, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	if err := d.ProcessOnce(ctx, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetOutbox(ctx, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusDelivered {
		t.Fatalf("after deliver %s", got.Status)
	}
	if sink.count() == 0 {
		t.Fatal("postgres dispatcher did not deliver")
	}
}

func TestPostgresOnlyLeaderDispatches(t *testing.T) {
	s := openOutboxPostgres(t)
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, `DELETE FROM ha_leader_lock WHERE lock_id='storage-server'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM ha_leader_lock WHERE lock_id='storage-server'`)
	})
	ok, err := s.TryAcquireLeader(ctx, "node-a", time.Minute)
	if err != nil || !ok {
		t.Fatalf("acquire leader: ok=%v err=%v", ok, err)
	}
	leaderA := ha.New(ha.Config{Enabled: true, NodeID: "node-a", LeaderTTL: time.Minute}, s)
	leaderB := ha.New(ha.Config{Enabled: true, NodeID: "node-b", LeaderTTL: time.Minute}, s)
	if !leaderA.IsLeader(ctx) {
		t.Fatal("node-a should be leader")
	}
	if leaderB.IsLeader(ctx) {
		t.Fatal("node-b should not be leader")
	}

	now := time.Now().UTC()
	rec := events.NormalizeOutbox(events.OutboxRecord{
		EventType: "ObjectCreated",
		Bucket:    "pg-ha",
		Key:       "obj",
		Attempts: map[string]events.SinkAttempt{
			"webhook": {Status: events.StatusPending, NextAttemptAt: now},
		},
	}, now)
	if _, err := s.InsertOutbox(ctx, rec); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM event_outbox WHERE id=$1`, rec.ID)
	})

	sinkA := &pgRecordingSink{name: "webhook"}
	sinkB := &pgRecordingSink{name: "webhook"}
	cfg := events.Config{MaxAttempts: 3, Retention: time.Hour, DeadRetention: time.Hour}
	dA := events.NewDispatcher(s, []events.EventSink{sinkA}, cfg, leaderA.IsLeader, events.Observer{})
	dB := events.NewDispatcher(s, []events.EventSink{sinkB}, cfg, leaderB.IsLeader, events.Observer{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = dB.ProcessOnce(ctx, now.Add(time.Second))
	}()
	go func() {
		defer wg.Done()
		_ = dA.ProcessOnce(ctx, now.Add(time.Second))
	}()
	wg.Wait()

	if sinkB.count() != 0 {
		t.Fatalf("follower delivered %d", sinkB.count())
	}
	if sinkA.count() != 1 {
		t.Fatalf("leader delivered %d", sinkA.count())
	}
	got, err := s.GetOutbox(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusDelivered {
		t.Fatalf("status %s", got.Status)
	}
}

func TestPostgresPutObjectWithOutboxAtomic(t *testing.T) {
	s := openOutboxPostgres(t)
	ctx := context.Background()
	bucket := "pg-ev-" + time.Now().UTC().Format("150405")
	if err := s.PutBucket(metadata.BucketRecord{Name: bucket, Owner: "tester", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM objects WHERE bucket=$1`, bucket)
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM buckets WHERE name=$1`, bucket)
	})
	now := time.Now().UTC()
	ev := events.NormalizeOutbox(events.OutboxRecord{
		EventType: metadata.EventObjectCreated,
		Bucket:    bucket,
		Key:       "a.txt",
		Attempts: map[string]events.SinkAttempt{
			"webhook": {Status: events.StatusPending, NextAttemptAt: now},
		},
	}, now)
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM event_outbox WHERE id=$1`, ev.ID)
	})
	_, err := s.PutObjectWithOutbox(metadata.ObjectRecord{
		Bucket: bucket, Key: "a.txt", Size: 3, ETag: "abc", ContentType: "text/plain", LastModified: now,
	}, []events.OutboxRecord{ev})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetObject(bucket, "a.txt"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetOutbox(ctx, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusPending || got.Key != "a.txt" {
		t.Fatalf("%+v", got)
	}
}

type pgRecordingSink struct {
	mu   sync.Mutex
	name string
	n    int
}

func (p *pgRecordingSink) Name() string { return p.name }
func (p *pgRecordingSink) Publish(context.Context, events.Event) error {
	p.mu.Lock()
	p.n++
	p.mu.Unlock()
	return nil
}
func (p *pgRecordingSink) Close() error { return nil }
func (p *pgRecordingSink) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n
}
