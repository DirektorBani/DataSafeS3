package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

// EventSink delivers S3-style notifications to external systems.
type EventSink interface {
	Publish(event string, payload map[string]any) error
	Name() string
}

type webhookEventSink struct {
	server *Server
}

func (w *webhookEventSink) Name() string { return "webhook" }

func (w *webhookEventSink) Publish(event string, payload map[string]any) error {
	w.server.fireWebhooks(event, payload)
	return nil
}

type rabbitMQEventSink struct{}

func (r *rabbitMQEventSink) Name() string { return "rabbitmq" }
func (r *rabbitMQEventSink) Publish(event string, payload map[string]any) error {
	return nil
}

type kafkaEventSink struct{}

func (k *kafkaEventSink) Name() string { return "kafka" }
func (k *kafkaEventSink) Publish(event string, payload map[string]any) error {
	return nil
}

func (s *Server) wireEventSinks() {
	s.eventSinks = []EventSink{
		&webhookEventSink{server: s},
		newNATSEventSink(),
		&rabbitMQEventSink{},
		&kafkaEventSink{},
	}
}

func (s *Server) emitEvent(event string, payload map[string]any) {
	for _, sink := range s.eventSinks {
		go func(sk EventSink) {
			_ = sk.Publish(event, payload)
		}(sink)
	}
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
	observability.SetClusterMetrics(len(nodes), healthy, offline)
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
