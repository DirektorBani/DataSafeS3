package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

const (
	EventObjectCreated = metadata.EventObjectCreated
	EventObjectDeleted = metadata.EventObjectDeleted
	EventBucketCreated = metadata.EventBucketCreated
)

func (s *Server) fireWebhooks(event string, payload map[string]any) {
	hooks, err := s.meta.ListWebhooks()
	if err != nil {
		return
	}
	body := map[string]any{
		"event":     event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      payload,
	}
	data, _ := json.Marshal(body)
	for _, h := range hooks {
		if !h.Enabled {
			continue
		}
		if !webhookMatchesEvent(h.Events, event) {
			continue
		}
		payloadStr := string(data)
		delivery := metadata.WebhookDeliveryRecord{
			ID: randomID(), WebhookID: h.ID, Event: event, URL: h.URL,
			Payload: payloadStr, CreatedAt: time.Now().UTC(), Attempts: 0,
		}
		go s.deliverWebhook(h, payloadStr, &delivery)
	}
}

func webhookMatchesEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event || e == "*" {
			return true
		}
	}
	return false
}

func (s *Server) deliverWebhook(hook metadata.WebhookRecord, payload string, delivery *metadata.WebhookDeliveryRecord) {
	delivery.Attempts++
	delivery.LastAttempt = time.Now().UTC()
	req, err := http.NewRequest(http.MethodPost, hook.URL, bytes.NewReader([]byte(payload)))
	if err != nil {
		delivery.Success = false
		delivery.Error = err.Error()
		_ = s.meta.PutWebhookDelivery(*delivery)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Datasafe-Webhook/1.0")
	for k, v := range hook.Headers {
		if k != "" {
			req.Header.Set(k, v)
		}
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		delivery.Success = false
		delivery.Error = err.Error()
		_ = s.meta.PutWebhookDelivery(*delivery)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	delivery.StatusCode = resp.StatusCode
	delivery.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !delivery.Success {
		delivery.Error = resp.Status
	}
	_ = s.meta.PutWebhookDelivery(*delivery)
}
