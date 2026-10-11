package events

import (
	"context"
	"testing"
	"time"
)

type scriptedHealth struct {
	name   string
	status string
	last   string
}

func (s *scriptedHealth) Name() string                         { return s.name }
func (s *scriptedHealth) Publish(context.Context, Event) error { return nil }
func (s *scriptedHealth) Close() error                         { return nil }
func (s *scriptedHealth) Health(context.Context) (string, string) {
	return s.status, s.last
}

func TestSinkHealthActivityIsFirstFailureAndRecovery(t *testing.T) {
	sk := &scriptedHealth{name: "webhook", status: "degraded", last: WaitingForConnection}
	var degraded []bool
	var removed []string
	var changes []string
	store := newMemOutbox()
	d := NewDispatcher(store, []EventSink{sk}, Config{MaxAttempts: 3, Retention: time.Hour, DeadRetention: time.Hour}, nil, Observer{
		Degraded: func(name string, on bool) {
			degraded = append(degraded, on)
		},
		SinkRemoved: func(name string) { removed = append(removed, name) },
		Status: func(name, status, last string) {
			changes = append(changes, name+"|"+status+"|"+last)
		},
	})
	d.purgeEvery = time.Hour
	now := time.Now().UTC()
	d.lastPurge = now
	ctx := context.Background()
	if err := d.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("waiting sentence wrote activity %v", changes)
	}
	if len(degraded) != 1 || !degraded[0] {
		t.Fatalf("gauge %v", degraded)
	}

	sk.last = "dial tcp 192.0.2.1:9092: connection refused"
	if err := d.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	sk.last = "dial tcp 192.0.2.1:9092: i/o timeout"
	if err := d.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0] != "webhook|degraded|dial tcp 192.0.2.1:9092: connection refused" {
		t.Fatalf("changes %v", changes)
	}

	sk.status, sk.last = "connected", ""
	if err := d.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	sk.status, sk.last = "degraded", "HTTP 500"
	if err := d.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := d.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"webhook|degraded|dial tcp 192.0.2.1:9092: connection refused",
		"webhook|connected|",
		"webhook|degraded|HTTP 500",
	}
	if len(changes) != len(want) {
		t.Fatalf("changes %v", changes)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("changes %v", changes)
		}
	}
	d.Unregister("webhook")
	if len(removed) != 1 || removed[0] != "webhook" {
		t.Fatalf("removed %v", removed)
	}
}
