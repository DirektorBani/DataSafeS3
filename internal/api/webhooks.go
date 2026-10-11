package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

const (
	EventObjectCreated = metadata.EventObjectCreated
	EventObjectDeleted = metadata.EventObjectDeleted
	EventBucketCreated = metadata.EventBucketCreated
)

func (s *Server) deliverWebhooks(ctx context.Context, e events.Event) error {
	hooks, err := s.meta.ListWebhooks()
	if err != nil {
		return err
	}
	ts := e.CreatedAt
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	body := map[string]any{
		"event":     e.Type,
		"timestamp": ts.UTC().Format(time.RFC3339),
		"data":      e.Payload,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var errs []error
	for _, h := range hooks {
		if !h.Enabled || !webhookMatchesEvent(h.Events, e.Type) {
			continue
		}
		payloadStr := string(data)
		delivery := metadata.WebhookDeliveryRecord{
			ID: randomID(), WebhookID: h.ID, Event: e.Type, URL: h.URL,
			Payload: payloadStr, CreatedAt: time.Now().UTC(), Attempts: 0,
		}
		if err := s.deliverWebhook(ctx, h, payloadStr, &delivery); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func webhookMatchesEvent(events []string, event string) bool {
	for _, e := range events {
		if e == event || e == "*" {
			return true
		}
	}
	return false
}

func (s *Server) deliverWebhook(ctx context.Context, hook metadata.WebhookRecord, payload string, delivery *metadata.WebhookDeliveryRecord) error {
	delivery.Attempts++
	delivery.LastAttempt = time.Now().UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader([]byte(payload)))
	if err != nil {
		delivery.Success = false
		delivery.Error = err.Error()
		_ = s.meta.PutWebhookDelivery(*delivery)
		return err
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
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	delivery.StatusCode = resp.StatusCode
	delivery.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !delivery.Success {
		delivery.Error = resp.Status
	}
	_ = s.meta.PutWebhookDelivery(*delivery)
	if !delivery.Success {
		return fmt.Errorf("webhook %s: %s", hook.ID, resp.Status)
	}
	return nil
}
