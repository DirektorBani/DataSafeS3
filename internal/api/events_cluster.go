package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

type webhookEventSink struct {
	server *Server
}

func (w *webhookEventSink) Name() string { return "webhook" }

func (w *webhookEventSink) Publish(ctx context.Context, e events.Event) error {
	return w.server.deliverWebhooks(ctx, e)
}

func (w *webhookEventSink) Close() error { return nil }

// defaultEventSinks is webhook + NATS only. RabbitMQ and Kafka no-op sinks are not wired.
// Kafka (E02) should Dispatcher.Register when STORAGE_KAFKA_BROKERS is non-empty.
func defaultEventSinks(s *Server) []events.EventSink {
	return []events.EventSink{
		&webhookEventSink{server: s},
		newNATSEventSink(),
	}
}

func (s *Server) wireEventSinks() {
	cfg := events.ConfigFromEnv()
	s.eventCfg = cfg
	if m, ok := s.meta.(metadata.OutboxMutator); ok {
		m.SetOutboxMaxPending(cfg.MaxPending)
	}
	s.eventDisp = events.NewDispatcher(s.outboxStore(), defaultEventSinks(s), cfg, s.isEventLeader, events.Observer{
		Pending:   observability.SetEventsOutboxPending,
		Delivered: observability.AddEventsDelivered,
		Failed:    observability.AddEventsFailed,
		Dead:      observability.AddEventsDead,
		Lag:       observability.ObserveEventsDeliveryLag,
	})
}

func (s *Server) outboxStore() events.OutboxStore {
	st, _ := s.meta.(events.OutboxStore)
	return st
}

func (s *Server) isEventLeader(ctx context.Context) bool {
	if s.haLeader == nil || !s.haLeader.Enabled() {
		return true
	}
	return s.haLeader.IsLeader(ctx)
}

func (s *Server) outboxRecord(eventType, bucket, key, versionID, tenantID string, payload map[string]any) events.OutboxRecord {
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{}`)
	}
	now := time.Now().UTC()
	rec := events.OutboxRecord{
		EventType: eventType,
		Bucket:    bucket,
		Key:       key,
		VersionID: versionID,
		TenantID:  tenantID,
		Payload:   raw,
		CreatedAt: now,
		Attempts:  map[string]events.SinkAttempt{},
	}
	var names []string
	if s.eventDisp != nil {
		names = s.eventDisp.SinkNames()
	}
	for _, name := range names {
		rec.Attempts[name] = events.SinkAttempt{Status: events.StatusPending, NextAttemptAt: now}
	}
	return events.NormalizeOutbox(rec, now)
}

func (s *Server) noteOutboxDropped(n int) {
	if n <= 0 {
		return
	}
	observability.AddEventsDropped(n, "pending_cap")
	msg := fmt.Sprintf("dropped %d pending event(s): %s", n, events.DropReasonCap)
	s.logActivityAs("system", "", metadata.ActionEventsOutboxDropped, "events", msg)
	slog.Warn("event outbox pending cap", "dropped", n, "reason", events.DropReasonCap)
}

type clusterMonitor struct {
	meta   metadata.MetadataStore
	mu     sync.RWMutex
	nodes  []metadata.ClusterNode
	status string
}

func newClusterMonitor(meta metadata.MetadataStore) *clusterMonitor {
	return &clusterMonitor{meta: meta, status: "healthy"}
}

func (c *clusterMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	c.probe()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.probe()
		}
	}
}

func (c *clusterMonitor) probe() {
	cfg, err := c.meta.GetSystemConfig()
	if err != nil {
		return
	}
	nodes := cfg.Cluster.Nodes
	if len(nodes) == 0 {
		nodes = []metadata.ClusterNode{{
			ID: "local", Address: "localhost:9000", Role: "primary", Status: "healthy",
		}}
	}
	healthy, offline := 0, 0
	samples := make([]observability.ClusterNodeSample, 0, len(nodes))
	for i := range nodes {
		status := c.checkNode(nodes[i].Address)
		nodes[i].Status = status
		switch status {
		case "healthy":
			healthy++
		case "offline":
			offline++
		default:
			// degraded counted as neither fully healthy nor offline
		}
		samples = append(samples, observability.ClusterNodeSample{
			ID:      nodes[i].ID,
			Address: nodes[i].Address,
			Role:    nodes[i].Role,
			Status:  status,
		})
	}
	overall := "healthy"
	if offline > 0 && healthy == 0 {
		overall = "offline"
	} else if offline > 0 || healthy < len(nodes) {
		overall = "degraded"
	}
	c.mu.Lock()
	c.nodes = nodes
	c.status = overall
	c.mu.Unlock()
	observability.SetClusterNodeStatuses(overall, samples)
}

func (c *clusterMonitor) checkNode(address string) string {
	url := address
	if !hasHTTPPrefix(url) {
		url = "http://" + url
	}
	url += "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "offline"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "degraded"
	}
	var body struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Status == "ok" {
		return "healthy"
	}
	return "degraded"
}

func hasHTTPPrefix(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || (len(s) > 8 && s[:8] == "https://"))
}

func (c *clusterMonitor) Snapshot() (string, []metadata.ClusterNode) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	nodes := make([]metadata.ClusterNode, len(c.nodes))
	copy(nodes, c.nodes)
	return c.status, nodes
}
