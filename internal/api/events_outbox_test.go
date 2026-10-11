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

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestDefaultSinksDoNotIncludeBrokerStubs(t *testing.T) {
	t.Setenv("STORAGE_NATS_URL", "")
	meta, err := metadata.OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	server := &Server{meta: meta}
	server.eventDisp = events.NewDispatcher(meta, nil, events.ConfigFromEnv(), nil, events.Observer{})
	server.syncEventSinks()
	for _, name := range server.eventDisp.SinkNames() {
		switch name {
		case "kafka", "rabbitmq", "nats", "webhook":
			t.Fatalf("unconfigured sink %q is registered", name)
		default:
			t.Fatalf("unexpected sink %q", name)
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
	server.eventDisp = events.NewDispatcher(meta, nil, events.Config{
		MaxAttempts: 3, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	server.syncEventSinks()
	recs := server.outboxRecords("ObjectCreated", "b", "k", "", "", map[string]any{"bucket": "b", "key": "k"})
	if len(recs) != 1 {
		t.Fatalf("rows %d", len(recs))
	}
	now := time.Now().UTC()
	if _, err := meta.InsertOutbox(context.Background(), recs[0]); err != nil {
		t.Fatal(err)
	}
	if err := server.eventDisp.ProcessOnce(context.Background(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("webhook hits %d", hits.Load())
	}
	got, err := meta.GetOutbox(context.Background(), recs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != events.StatusDelivered {
		t.Fatalf("status %s attempts %+v", got.Status, got.Attempts)
	}
	items, err := meta.ListWebhookDeliveries("wh1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Attempts != 1 || !items[0].Success {
		t.Fatalf("deliveries %+v", items)
	}
}

func TestWebhookRetriesUpdateOneDeliveryRecord(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "down", http.StatusBadGateway)
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
	server.eventDisp = events.NewDispatcher(meta, nil, events.Config{
		MaxAttempts: 5, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{})
	server.syncEventSinks()
	recs := server.outboxRecords("ObjectCreated", "b", "k", "", "", map[string]any{"bucket": "b", "key": "k"})
	if len(recs) != 1 {
		t.Fatalf("rows %d", len(recs))
	}
	if _, err := meta.InsertOutbox(context.Background(), recs[0]); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := server.eventDisp.ProcessOnce(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if err := server.eventDisp.ProcessOnce(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	items, err := meta.ListWebhookDeliveries("wh1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("delivery rows %d", len(items))
	}
	if items[0].Attempts != 2 || items[0].Success || items[0].StatusCode != http.StatusBadGateway {
		t.Fatalf("after dispatcher retries %+v", items[0])
	}
	if items[0].ID != recs[0].ID+":wh1" {
		t.Fatalf("delivery id %s", items[0].ID)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/wh1/deliveries/"+items[0].ID+"/retry", nil)
	req.SetPathValue("id", "wh1")
	req.SetPathValue("deliveryId", items[0].ID)
	rr := httptest.NewRecorder()
	server.handleRetryWebhookDelivery(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("retry status %d %s", rr.Code, rr.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	var last metadata.WebhookDeliveryRecord
	for time.Now().Before(deadline) {
		got, err := meta.ListWebhookDeliveries("wh1", 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 1 && got[0].Attempts == 3 {
			last = got[0]
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if last.Attempts != 3 || last.ID != items[0].ID {
		t.Fatalf("manual retry did not update the same row: %+v hits=%d", last, hits.Load())
	}
	if hits.Load() != 3 {
		t.Fatalf("http attempts %d", hits.Load())
	}
}

func TestSingleNodeWithoutSinksWritesNoOutbox(t *testing.T) {
	t.Setenv("STORAGE_NATS_URL", "")
	t.Setenv("STORAGE_METADATA_BACKEND", "bolt")
	beforeNATS, natsSeries := deliveredSink("nats")
	beforeHook, hookSeries := deliveredSink("webhook")

	dir := t.TempDir()
	srv, err := NewServer(Config{
		DataDir: dir, Region: "us-east-1",
		AccessKey: "testkey", SecretKey: "testsecret",
		AdminUser: "admin", AdminPassword: "admin",
		JWTSecret: "test-jwt-secret",
		Metadata:  metadata.Config{Backend: "bolt", DataDir: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	if names := srv.eventDisp.SinkNames(); len(names) != 0 {
		t.Fatalf("sinks %v", names)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/buckets/plain", nil)
	req.SetPathValue("bucket", "plain")
	req = withAuth(req, auth.TokenInfo{Username: "admin", Role: auth.RoleAdministrator, UserID: "admin-bootstrap"})
	rr := httptest.NewRecorder()
	srv.handleCreateBucketJSON(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rr.Code, rr.Body.String())
	}
	if n := outboxTotal(t, srv); n != 0 {
		t.Fatalf("outbox rows %d", n)
	}
	if err := srv.eventDisp.ProcessOnce(context.Background(), time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	afterNATS, natsAfter := deliveredSink("nats")
	afterHook, hookAfter := deliveredSink("webhook")
	if natsAfter && (!natsSeries || afterNATS != beforeNATS) {
		t.Fatalf("nats delivered series before=%v %v after=%v %v", natsSeries, beforeNATS, natsAfter, afterNATS)
	}
	if hookAfter && (!hookSeries || afterHook != beforeHook) {
		t.Fatalf("webhook delivered series before=%v %v after=%v %v", hookSeries, beforeHook, hookAfter, afterHook)
	}

	if err := srv.meta.PutWebhook(metadata.WebhookRecord{
		ID: "later", Name: "later", URL: "http://127.0.0.1:9/hook", Events: []string{"*"}, Enabled: true, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	srv.syncEventSinks()
	if !srv.eventDisp.HasSink("webhook") {
		t.Fatal("webhook sink not registered after enable")
	}
	if n := outboxTotal(t, srv); n != 0 {
		t.Fatalf("enabling a webhook replayed %d rows", n)
	}
}

func outboxTotal(t *testing.T, srv *Server) int {
	t.Helper()
	st := srv.outboxStore()
	if st == nil {
		t.Fatal("no outbox store")
	}
	ctx := context.Background()
	total := 0
	for _, status := range []string{events.StatusPending, events.StatusDelivered, events.StatusDead, events.StatusDropped} {
		n, err := st.CountOutbox(ctx, status)
		if err != nil {
			t.Fatal(err)
		}
		total += n
	}
	return total
}

func deliveredSink(sink string) (float64, bool) {
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0, false
	}
	for _, mf := range families {
		if mf.GetName() != "datasafe_events_delivered_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelValue(m, "sink") == sink {
				if c := m.GetCounter(); c != nil {
					return c.GetValue(), true
				}
			}
		}
	}
	return 0, false
}

func labelValue(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}

type activityHealthSink struct {
	status string
	last   string
}

func (s *activityHealthSink) Name() string                                { return "webhook" }
func (s *activityHealthSink) Publish(context.Context, events.Event) error { return nil }
func (s *activityHealthSink) Close() error                                { return nil }
func (s *activityHealthSink) Health(context.Context) (string, string) {
	return s.status, s.last
}

func TestEventSinkFailureAndRecoveryWriteActivity(t *testing.T) {
	meta, err := metadata.OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer meta.Close()
	server := &Server{meta: meta}
	sk := &activityHealthSink{status: "degraded", last: events.WaitingForConnection}
	server.eventDisp = events.NewDispatcher(meta, []events.EventSink{sk}, events.Config{
		MaxAttempts: 3, Retention: time.Hour, DeadRetention: time.Hour,
	}, nil, events.Observer{Status: server.noteEventSinkStatus})
	now := time.Now().UTC()
	ctx := context.Background()
	if err := server.eventDisp.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	sk.last = "HTTP 502"
	if err := server.eventDisp.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := server.eventDisp.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	sk.status, sk.last = "connected", ""
	if err := server.eventDisp.ProcessOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	got, err := meta.ListActivity(metadata.ActivityFilter{Period: "all", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, ev := range got.Events {
		if ev.Action == metadata.ActionEventsSinkDegraded || ev.Action == metadata.ActionEventsSinkRecovered {
			seen[ev.Action+":"+ev.ResourceName]++
		}
	}
	if seen[metadata.ActionEventsSinkDegraded+":webhook: HTTP 502"] != 1 || seen[metadata.ActionEventsSinkRecovered+":webhook"] != 1 || len(seen) != 2 {
		t.Fatalf("activity %v", seen)
	}
}
