package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	terminalOnly := true
	var retryAfter time.Duration
	for _, h := range hooks {
		if !h.Enabled || !webhookMatchesEvent(h.Events, e.Type) {
			continue
		}
		payloadStr := string(data)
		delivery := s.webhookDeliveryFor(h, e, payloadStr)
		err := s.deliverWebhook(ctx, h, payloadStr, &delivery)
		if err == nil {
			continue
		}
		errs = append(errs, err)
		var terminal *events.TerminalError
		if !errors.As(err, &terminal) {
			terminalOnly = false
		}
		var ra *events.RetryAfterError
		if errors.As(err, &ra) && ra.After > retryAfter {
			retryAfter = ra.After
		}
	}
	if len(errs) == 0 {
		return nil
	}
	joined := errors.Join(errs...)
	if terminalOnly {
		return &events.TerminalError{Err: joined}
	}
	if retryAfter > 0 {
		return &events.RetryAfterError{Err: joined, After: retryAfter}
	}
	return joined
}

// webhookDeliveryID is stable for one outbox event and one webhook, so retries
// update the same console history row.
func webhookDeliveryID(eventID, webhookID string) string {
	if eventID == "" || webhookID == "" {
		return randomID()
	}
	return eventID + ":" + webhookID
}

func outboxIDFromDelivery(deliveryID, webhookID string) (string, bool) {
	suffix := ":" + webhookID
	if webhookID == "" || !strings.HasSuffix(deliveryID, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(deliveryID, suffix)
	if id == "" {
		return "", false
	}
	return id, true
}

func (s *Server) webhookDeliveryFor(h metadata.WebhookRecord, e events.Event, payload string) metadata.WebhookDeliveryRecord {
	id := webhookDeliveryID(e.ID, h.ID)
	if existing, err := s.meta.GetWebhookDelivery(id); err == nil && existing.WebhookID == h.ID {
		existing.Event = e.Type
		existing.URL = h.URL
		existing.Payload = payload
		return existing
	}
	return metadata.WebhookDeliveryRecord{
		ID: id, WebhookID: h.ID, Event: e.Type, URL: h.URL,
		Payload: payload, CreatedAt: time.Now().UTC(), Attempts: 0,
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

func (s *Server) deliverWebhook(ctx context.Context, hook metadata.WebhookRecord, payload string, delivery *metadata.WebhookDeliveryRecord) error {
	delivery.Attempts++
	delivery.LastAttempt = time.Now().UTC()
	delivery.Error = ""
	delivery.StatusCode = 0
	delivery.Success = false
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader([]byte(payload)))
	if err != nil {
		delivery.Success = false
		delivery.Error = err.Error()
		_ = s.meta.PutWebhookDelivery(*delivery)
		return &events.TerminalError{Err: err}
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
	if delivery.Success {
		return nil
	}
	return classifyHTTPStatus(resp.StatusCode, fmt.Sprintf("webhook %s: %s", hook.ID, resp.Status), resp.Header.Get("Retry-After"), "")
}

// classifyHTTPStatus maps a response to a retry, a capped Retry-After, or a terminal 4xx.
// Transport errors stay with the caller. 408, 429, and 5xx retry. Any other
// non-2xx, including the rest of 4xx and 3xx, is terminal. label prefixes the
// error when the status string is only the reason (bucket notifications).
func classifyHTTPStatus(code int, status, retryAfter, label string) error {
	if code >= 200 && code < 300 {
		return nil
	}
	msg := status
	if label != "" {
		msg = label + ": " + status
	}
	err := fmt.Errorf("%s", msg)
	if code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500 {
		if d := parseRetryAfter(retryAfter); d > 0 {
			return &events.RetryAfterError{Err: err, After: d}
		}
		return err
	}
	return &events.TerminalError{Err: err}
}

func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if sec, err := strconv.Atoi(h); err == nil {
		if sec <= 0 {
			return 0
		}
		return time.Duration(sec) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0
		}
		return d
	}
	return 0
}
