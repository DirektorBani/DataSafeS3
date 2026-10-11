package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestDefaultSinksDoNotIncludeBrokerStubs(t *testing.T) {
	t.Setenv("STORAGE_NATS_URL", "")
	for _, sk := range defaultEventSinks(&Server{}) {
		switch sk.Name() {
		case "kafka", "rabbitmq":
			t.Fatalf("noop broker sink %q is still wired", sk.Name())
		}
	}
	n := newNATSEventSink()
	if err := n.Publish(context.Background(), events.Event{Type: "ObjectCreated"}); err != nil {
		t.Fatal(err)
	}
	if n.nc != nil {
		t.Fatal("empty STORAGE_NATS_URL opened a NATS connection")
	}
}

func TestWebhookDeliveredFromOutbox(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Errorf("empty body")
		}
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	meta, err := metadata.OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	if err := meta.PutWebhook(metadata.WebhookRecord{
		ID: "wh1", Name: "hook", URL: srv.URL, Events: []string{"ObjectCreated"}, Enabled: true, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	server := &Server{meta: meta}
	sink := &webhookEventSink{server: server}
	d := events.NewDispatcher(meta, []events.EventSink{sink}, events.Config{
		MaxAttempts: 3, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	now := time.Now().UTC()
	rec := server.outboxRecord("ObjectCreated", "b", "k", "", "", map[string]any{"bucket": "b", "key": "k"})
	// outboxRecord stamps sink names from eventDisp, which is nil here, so set the attempt explicitly.
	rec.Attempts = map[string]events.SinkAttempt{
		"webhook": {Status: events.StatusPending, NextAttemptAt: now},
	}
	rec.Status = events.StatusPending
	if _, err := meta.InsertOutbox(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	if err := d.ProcessOnce(context.Background(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("webhook hits %d", hits.Load())
	}
	got, err := meta.GetOutbox(context.Background(), rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusDelivered {
		t.Fatalf("status %s attempts %+v", got.Status, got.Attempts)
	}
}
