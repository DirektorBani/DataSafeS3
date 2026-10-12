package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestThreeNodesFreshPostgresBootstrapAndWebhookTakeover(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	t.Setenv("STORAGE_HA_ENABLED", "true")
	t.Setenv("STORAGE_HA_LEADER_TTL", "2s")
	t.Setenv("STORAGE_NATS_URL", "")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_NODE_ID", "")

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("ds_ha_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close()
	})
	fresh, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Path = "/" + name

	const nodes = 3
	dirs := make([]string, nodes)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}
	servers := make([]*Server, nodes)
	startErr := make([]error, nodes)
	var wg sync.WaitGroup
	for i := 0; i < nodes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			srv, err := NewServer(Config{
				DataDir: dirs[i], AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test",
				NodeID: fmt.Sprintf("node-%d", i),
				Metadata: metadata.Config{
					Backend: metadata.BackendPostgres, PostgresDSN: fresh.String(),
				},
			})
			servers[i] = srv
			startErr[i] = err
		}(i)
	}
	wg.Wait()
	t.Cleanup(func() {
		for _, srv := range servers {
			if srv != nil {
				_ = srv.Close()
			}
		}
	})
	for i, err := range startErr {
		if err != nil {
			t.Fatalf("node %d failed to start: %v", i, err)
		}
	}
	n, err := servers[0].meta.CountUsers()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("admin users %d", n)
	}
	for _, srv := range servers {
		if srv.eventDisp.HasSink("webhook") {
			t.Fatal("webhook sink registered before a webhook existed")
		}
	}

	var hits atomic.Int32
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hook.Close()

	cancels := make([]context.CancelFunc, nodes)
	for i, srv := range servers {
		bg, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		srv.StartBackground(bg)
	}
	t.Cleanup(func() {
		for _, cancel := range cancels {
			if cancel != nil {
				cancel()
			}
		}
	})

	leader := waitLeader(t, servers, "")
	if err := leader.meta.PutWebhook(metadata.WebhookRecord{
		ID: "wh-cluster", Name: "cluster", URL: hook.URL, Events: []string{"ObjectCreated"},
		Enabled: true, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	insertWebhookEvent(t, leader, "before-takeover")
	waitWebhookHits(t, &hits, 1)

	oldID := leader.haLeader.NodeID()
	for i, srv := range servers {
		if srv.haLeader.NodeID() == oldID {
			cancels[i]()
			cancels[i] = nil
		}
	}
	next := waitLeader(t, servers, oldID)
	deadline := time.Now().Add(10 * time.Second)
	for !next.eventDisp.HasSink("webhook") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !next.eventDisp.HasSink("webhook") {
		t.Fatal("new leader has no webhook sink")
	}
	insertWebhookEvent(t, next, "after-takeover")
	waitWebhookHits(t, &hits, 2)
	dead, err := next.outboxStore().CountOutbox(context.Background(), events.StatusDead)
	if err != nil {
		t.Fatal(err)
	}
	if dead != 0 {
		t.Fatalf("dead outbox rows %d", dead)
	}
}

func waitLeader(t *testing.T, servers []*Server, notID string) *Server {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, srv := range servers {
			if srv == nil || srv.haLeader == nil {
				continue
			}
			if notID != "" && srv.haLeader.NodeID() == notID {
				continue
			}
			if srv.haLeader.IsLeader(context.Background()) {
				return srv
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("no leader (excluding %q)", notID)
	return nil
}

func insertWebhookEvent(t *testing.T, srv *Server, key string) {
	t.Helper()
	now := time.Now().UTC()
	rec := events.NormalizeOutbox(events.OutboxRecord{
		EventType: "ObjectCreated",
		Bucket:    "b",
		Key:       key,
		Payload:   []byte(`{"bucket":"b","key":"` + key + `"}`),
		CreatedAt: now,
		Attempts: map[string]events.SinkAttempt{
			"webhook": {Status: events.StatusPending, NextAttemptAt: now},
		},
	}, now)
	if _, err := srv.outboxStore().InsertOutbox(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func waitWebhookHits(t *testing.T, hits *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for hits.Load() < want && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
	}
	if hits.Load() < want {
		t.Fatalf("webhook deliveries %d, want %d", hits.Load(), want)
	}
}
