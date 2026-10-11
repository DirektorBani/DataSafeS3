package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/urlpolicy"
)

const (
	bucketWebhookSinkName = "bucket-webhook"
	bucketWebhookURLField = "_webhook_url"
)

// objectOutbox builds rows committed with an object mutation.
// Global ObjectCreated / ObjectDeleted rows are written only when the S3
// handler set WithObjectEvents and a sink is registered for that event.
// A matching per-bucket notification is its own row, including admin and
// internal puts that previously called EmitBucketNotification.
func (s *Server) objectOutbox(ctx context.Context, bucketRec metadata.BucketRecord, obj metadata.ObjectRecord, kind string) []events.OutboxRecord {
	if s == nil || s.eventDisp == nil {
		return nil
	}
	global := s3.ObjectEventsRequested(ctx)
	cfg, hasCfg := metadata.BucketNotificationFromTags(bucketRec.Tags)
	notify := hasCfg && metadata.BucketNotificationMatches(cfg, kind, obj.Key)
	if !global && !notify {
		return nil
	}
	logical := bucketRec.Name
	if logical == "" {
		logical = obj.Bucket
	}
	var out []events.OutboxRecord
	if global {
		switch kind {
		case "delete":
			out = append(out, s.outboxRecords(metadata.EventObjectDeleted, logical, obj.Key, obj.VersionID, bucketRec.TenantID, map[string]any{
				"bucket": logical, "key": obj.Key,
			})...)
		case "multipart":
			out = append(out, s.outboxRecords(metadata.EventObjectCreated, logical, obj.Key, obj.VersionID, bucketRec.TenantID, map[string]any{
				"bucket": logical, "key": obj.Key, "size": obj.Size,
			})...)
			out = append(out, s.outboxRecords(metadata.EventMultipartCompleted, logical, obj.Key, obj.VersionID, bucketRec.TenantID, map[string]any{
				"bucket": logical, "key": obj.Key, "size": obj.Size,
			})...)
		default:
			out = append(out, s.outboxRecords(metadata.EventObjectCreated, logical, obj.Key, obj.VersionID, bucketRec.TenantID, map[string]any{
				"bucket": logical, "key": obj.Key, "size": obj.Size,
			})...)
		}
	}
	if !notify {
		return out
	}
	s.ensureBucketWebhookSink()
	posted := metadata.ReplEventPut
	if kind == "delete" {
		posted = metadata.ReplEventDelete
	}
	var size int64
	if kind != "delete" {
		size = obj.Size
	}
	when := obj.LastModified
	if when.IsZero() {
		when = time.Now().UTC()
	}
	raw, err := json.Marshal(map[string]any{
		"event":               posted,
		"bucket":              obj.Bucket,
		"key":                 obj.Key,
		"size":                size,
		"time":                when.UTC().Format(time.RFC3339),
		bucketWebhookURLField: cfg.WebhookURL,
	})
	if err != nil {
		return out
	}
	now := time.Now().UTC()
	out = append(out, events.NormalizeOutbox(events.OutboxRecord{
		EventType: "BucketNotification",
		Bucket:    obj.Bucket,
		Key:       obj.Key,
		VersionID: obj.VersionID,
		TenantID:  bucketRec.TenantID,
		Payload:   raw,
		CreatedAt: now,
		Attempts: map[string]events.SinkAttempt{
			bucketWebhookSinkName: {Status: events.StatusPending, NextAttemptAt: now},
		},
	}, now))
	return out
}

func (s *Server) ensureBucketWebhookSink() {
	if s == nil || s.eventDisp == nil || s.eventDisp.HasSink(bucketWebhookSinkName) {
		return
	}
	s.eventDisp.Register(&bucketWebhookSink{})
}

func (s *Server) anyBucketNotification() bool {
	if s == nil || s.meta == nil {
		return false
	}
	buckets, err := s.meta.ListBuckets()
	if err != nil {
		return false
	}
	for _, b := range buckets {
		cfg, ok := metadata.BucketNotificationFromTags(b.Tags)
		if ok && cfg.WebhookURL != "" {
			return true
		}
	}
	return false
}

type bucketWebhookSink struct{}

func (b *bucketWebhookSink) Name() string { return bucketWebhookSinkName }

func (b *bucketWebhookSink) Close() error { return nil }

func (b *bucketWebhookSink) Publish(ctx context.Context, e events.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rawURL, _ := e.Payload[bucketWebhookURLField].(string)
	if rawURL == "" {
		return &events.TerminalError{Err: fmt.Errorf("bucket notification: missing url")}
	}
	if err := urlpolicy.ValidateOutboundURL(rawURL, urlpolicy.DefaultOptions()); err != nil {
		return &events.TerminalError{Err: fmt.Errorf("bucket notification: %s", urlpolicy.OutboundURLError(err))}
	}
	pub := make(map[string]any, len(e.Payload))
	for k, v := range e.Payload {
		if k == bucketWebhookURLField {
			continue
		}
		pub[k] = v
	}
	body, err := json.Marshal(pub)
	if err != nil {
		return &events.TerminalError{Err: err}
	}
	eventName, _ := pub["event"].(string)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return &events.TerminalError{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DataSafe-Event", eventName)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return classifyHTTPStatus(resp.StatusCode, resp.Status, resp.Header.Get("Retry-After"), "bucket notification")
}
