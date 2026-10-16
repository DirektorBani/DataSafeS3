package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

type webhookEventSink struct {
	server  *Server
	mu      sync.Mutex
	lastErr string
}

func (w *webhookEventSink) Name() string { return "webhook" }

func (w *webhookEventSink) Publish(ctx context.Context, e events.Event) error {
	err := w.server.deliverWebhooks(ctx, e)
	w.mu.Lock()
	if err == nil {
		w.lastErr = ""
	} else {
		w.lastErr = err.Error()
	}
	w.mu.Unlock()
	return err
}

func (w *webhookEventSink) Health(context.Context) (string, string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastErr != "" {
		return "degraded", w.lastErr
	}
	return "connected", ""
}

func (w *webhookEventSink) Close() error { return nil }

// eventSinkRefreshInterval is how often every node reloads sinks from the database.
// Webhook CRUD on one node is not visible to the others until this refresh, or
// until the node that just became leader runs syncEventSinks from the lease callback.
const eventSinkRefreshInterval = 2 * time.Second

// syncEventSinks registers a sink only when it is configured.
// NATS is registered when STORAGE_NATS_URL is non-empty.
// The webhook sink is registered when at least one webhook is enabled.
// Callers are webhook CRUD on this node, leadership gain, and the periodic refresh.
// Kafka is registered when STORAGE_KAFKA_BROKERS is non-empty.
func (s *Server) syncEventSinks() {
	if s == nil || s.eventDisp == nil {
		return
	}
	_ = s.syncKafkaSink()
	if natsURLConfigured() {
		if !s.eventDisp.HasSink("nats") {
			s.eventDisp.Register(newNATSEventSink())
		}
	} else {
		s.eventDisp.Unregister("nats")
	}
	if s.anyEnabledWebhook() {
		if !s.eventDisp.HasSink("webhook") {
			s.eventDisp.Register(&webhookEventSink{server: s})
		}
	} else {
		s.eventDisp.Unregister("webhook")
	}
	if s.anyBucketNotification() {
		s.ensureBucketWebhookSink()
	}
}

func (s *Server) anyEnabledWebhook() bool {
	if s == nil || s.meta == nil {
		return false
	}
	hooks, err := s.meta.ListWebhooks()
	if err != nil {
		return false
	}
	for _, h := range hooks {
		if h.Enabled {
			return true
		}
	}
	return false
}

// eventSinksFor lists sinks that should receive this event.
// A public share GET is allowed on a follower, which may not have registered
// the webhook sink yet. The row is still written so the leader can deliver it.
func (s *Server) eventSinksFor(eventType string) []string {
	if s == nil || s.eventDisp == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var names []string
	add := func(name string) {
		if name == "" || name == bucketWebhookSinkName {
			return
		}
		if name == "webhook" && !s.webhookMatchesConfigured(eventType) {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, name := range s.eventDisp.SinkNames() {
		add(name)
	}
	if s.webhookMatchesConfigured(eventType) {
		add("webhook")
	}
	if natsURLConfigured() {
		add("nats")
	}
	if kafkaBrokersConfigured() {
		add("kafka")
	}
	return names
}

func (s *Server) webhookMatchesConfigured(eventType string) bool {
	if s == nil || s.meta == nil {
		return false
	}
	hooks, err := s.meta.ListWebhooks()
	if err != nil {
		return false
	}
	for _, h := range hooks {
		if h.Enabled && webhookMatchesEvent(h.Events, eventType) {
			return true
		}
	}
	return false
}

func (s *Server) wireEventSinks() error {
	cfg := events.ConfigFromEnv()
	s.eventCfg = cfg
	if m, ok := s.meta.(metadata.OutboxMutator); ok {
		m.SetOutboxMaxPending(cfg.MaxPending)
	}
	s.eventDisp = events.NewDispatcher(s.outboxStore(), nil, cfg, s.isEventLeader, events.Observer{
		Pending:     observability.SetEventsOutboxPending,
		Delivered:   observability.AddEventsDelivered,
		Failed:      observability.AddEventsFailed,
		Dead:        observability.AddEventsDead,
		Lag:         observability.ObserveEventsDeliveryLag,
		Degraded:    observability.SetEventsSinkDegraded,
		SinkRemoved: observability.ClearEventsSinkDegraded,
		Status:      s.noteEventSinkStatus,
	})
	s.eventDisp.SetBeforePoll(s.withBackgroundDB)
	if err := s.syncKafkaSink(); err != nil {
		return err
	}
	s.syncEventSinks()
	s.noteKafkaBrokers()
	return nil
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

// outboxRecords builds the rows to commit with this metadata change.
// Sink names configured for this event are copied onto the row.
// A webhook name is included only when an enabled webhook matches eventType.
// An empty result means no outbox write: the event is not stored and is not
// replayed if a sink is configured later.
func (s *Server) outboxRecords(eventType, bucket, key, versionID, tenantID string, payload map[string]any) []events.OutboxRecord {
	if s == nil || s.eventDisp == nil {
		return nil
	}
	now := time.Now().UTC()
	attempts := map[string]events.SinkAttempt{}
	for _, name := range s.eventSinksFor(eventType) {
		attempts[name] = events.SinkAttempt{Status: events.StatusPending, NextAttemptAt: now}
	}
	if len(attempts) == 0 {
		return nil
	}
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{}`)
	}
	rec := events.NormalizeOutbox(events.OutboxRecord{
		EventType: eventType,
		Bucket:    bucket,
		Key:       key,
		VersionID: versionID,
		TenantID:  tenantID,
		Payload:   raw,
		CreatedAt: now,
		Attempts:  attempts,
	}, now)
	return []events.OutboxRecord{rec}
}

// noteEventSinkStatus writes one Activity row for a destination's first real
// failure and one when it recovers. The dispatcher suppresses retries.
func (s *Server) noteEventSinkStatus(sink, status, lastError string) {
	if s == nil || sink == "" {
		return
	}
	switch status {
	case "degraded":
		msg := sink
		if lastError != "" {
			msg = sink + ": " + trimActivityDetail(lastError, 240)
		}
		s.logActivityAs("system", "", metadata.ActionEventsSinkDegraded, "events", msg)
	case "connected":
		s.logActivityAs("system", "", metadata.ActionEventsSinkRecovered, "events", sink)
	}
}

func trimActivityDetail(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
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
	dbMu   *sync.Mutex
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
	cfg, err := c.systemConfig()
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

func (c *clusterMonitor) systemConfig() (metadata.SystemConfig, error) {
	if c.dbMu != nil {
		c.dbMu.Lock()
		defer c.dbMu.Unlock()
	}
	return c.meta.GetSystemConfig()
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
