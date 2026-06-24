package observability

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

// LogSink delivers structured log records to an external system.
type LogSink interface {
	Name() string
	Emit(record map[string]any) error
	Close() error
}

type SinkManager struct {
	mu    sync.RWMutex
	sinks []LogSink
}

var globalSinkManager = &SinkManager{}

func GlobalSinkManager() *SinkManager {
	return globalSinkManager
}

func (m *SinkManager) Reconfigure(cfg metadata.LoggingConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sinks {
		_ = s.Close()
	}
	m.sinks = buildLogSinks(cfg)
}

func (m *SinkManager) Emit(record map[string]any) {
	m.mu.RLock()
	sinks := append([]LogSink(nil), m.sinks...)
	m.mu.RUnlock()
	for _, s := range sinks {
		go func(sk LogSink) {
			_ = sk.Emit(record)
		}(s)
	}
}

func buildLogSinks(cfg metadata.LoggingConfig) []LogSink {
	var out []LogSink
	out = append(out, buildPlatformLogSinks(cfg)...)
	if cfg.Loki.Enabled {
		out = append(out, &lokiSink{cfg: cfg.Loki})
	}
	if cfg.Elasticsearch.Enabled {
		out = append(out, &elasticSink{cfg: cfg.Elasticsearch})
	}
	if cfg.Webhook.Enabled {
		out = append(out, &webhookLogSink{cfg: cfg.Webhook})
	}
	return out
}

type lokiSink struct {
	cfg metadata.LogSinkEndpoint
}

func (s *lokiSink) Name() string { return "loki" }
func (s *lokiSink) Close() error { return nil }
func (s *lokiSink) Emit(record map[string]any) error {
	ts := strconv.FormatInt(time.Now().UnixNano(), 10)
	msg, _ := json.Marshal(record)
	payload := map[string]any{
		"streams": []map[string]any{{
			"stream": map[string]string{"job": "datasafe"},
			"values": [][]string{{ts, string(msg)}},
		}},
	}
	body, _ := json.Marshal(payload)
	url := strings.TrimRight(s.cfg.Address, "/") + "/loki/api/v1/push"
	return postJSON(url, body, s.cfg, authBearer)
}

type elasticSink struct {
	cfg metadata.LogSinkEndpoint
}

func (s *elasticSink) Name() string { return "elasticsearch" }
func (s *elasticSink) Close() error { return nil }
func (s *elasticSink) Emit(record map[string]any) error {
	index := s.cfg.Index
	if index == "" {
		index = "datasafe-logs"
	}
	url := strings.TrimRight(s.cfg.Address, "/") + "/" + index + "/_doc"
	body, _ := json.Marshal(record)
	return postJSON(url, body, s.cfg, authApiKey)
}

type webhookLogSink struct {
	cfg metadata.LogSinkEndpoint
}

func (s *webhookLogSink) Name() string { return "webhook" }
func (s *webhookLogSink) Close() error { return nil }
func (s *webhookLogSink) Emit(record map[string]any) error {
	body, _ := json.Marshal(record)
	return postJSON(s.cfg.Address, body, s.cfg, authBearer)
}

type sinkAuthStyle int

const (
	authBearer sinkAuthStyle = iota
	authApiKey
)

func postJSON(url string, body []byte, cfg metadata.LogSinkEndpoint, style sinkAuthStyle) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		switch style {
		case authApiKey:
			req.Header.Set("Authorization", "ApiKey "+cfg.Token)
		default:
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
	} else if cfg.Username != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	if cfg.TLS {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("log sink HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
