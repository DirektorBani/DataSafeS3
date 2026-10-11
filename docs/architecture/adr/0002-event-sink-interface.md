# ADR 0002: EventSink interface and durable outbox

**Status:** Accepted  
**Date:** 2026-07-10 (proposed) · 2026-10-11 (accepted)  
**Related:** E01 durable outbox · E02 Kafka sink · E03 S3 events, bucket notifications, webhook classification, NATS JetStream

## Context

NATS notifications exist (`STORAGE_NATS_URL`). Platform teams also ask for Kafka. Delivery used to be `go sk.Publish(...)` with the error discarded, so a restart or a down receiver lost the event. A second, unused `EventSink` lived under `internal/events` (ADR-0002 stub) while `internal/api` defined another interface and wired no-op RabbitMQ and Kafka sinks that always succeeded.

## Decision

One interface, `internal/events.EventSink`:

```go
Name() string
Publish(ctx context.Context, e Event) error
Close() error
```

`Publish` returns an error when the attempt failed and should be retried. A disabled sink (NATS with an empty `STORAGE_NATS_URL`) returns nil and does not dial.

Events are written to a metadata outbox (bolt buckets `event_outbox` / `event_outbox_due`, Postgres table `event_outbox`, migration `020`) in the same bolt or Postgres transaction as the metadata change when that store method exists. A single `events.Dispatcher` per process delivers due rows. In HA (`STORAGE_HA_ENABLED` with the Postgres metadata backend) only the process that holds the existing leader lease delivers. Backoff is exponential from 1s, capped at 5 minutes, with ±20% jitter. Each sink has its own attempt counter. After `STORAGE_EVENTS_MAX_ATTEMPTS` (default 20) that sink is `dead`. Delivered rows are deleted after `STORAGE_EVENTS_RETENTION` (default 24h). Dead rows, and rows the pending cap marks `dropped`, are deleted after `STORAGE_EVENTS_DEAD_RETENTION` (default 168h). `STORAGE_EVENTS_MAX_PENDING` (default 100000, `0` = unlimited) drops the oldest pending rows instead of failing the object write. Dropped rows record reason `pending cap exceeded`, increment `datasafe_events_dropped_total`, and write Activity action `events_outbox_dropped`.

Webhook and NATS are adapters on this dispatcher. A sink is registered only when configured: NATS when `STORAGE_NATS_URL` is set, webhook when at least one webhook is enabled (refreshed on webhook changes), bucket-notification when a bucket has a notification URL. An event whose snapshot is empty is not stored and is not replayed. `Publish` may return `*events.TerminalError` (the sink is dead immediately; used for HTTP 4xx other than 408 and 429, and for 3xx) or `*events.RetryAfterError` (retried, waiting at least `After`, capped at 5 minutes). NATS publishes with a JetStream ack when the connected server has JetStream and a stream covers the subject; otherwise it is core publish, which cannot confirm subscriber delivery. S3 SigV4 object events and bucket-notification webhooks are written through the same row and `*WithOutbox` helpers. That is required for Kafka. The row and `Dispatcher.Register` are the extension point.

Kafka uses `github.com/twmb/franz-go` (pure Go, no CGO). That client is the owner-approved choice. The container image builds the server with `-tags datasafe_sinkexec` and ships the client as the static program `/usr/lib/datasafe/datasafe-kafka-bridge`. The server starts that program only when `STORAGE_KAFKA_BROKERS` is non-empty, so an idle process does not map franz-go and does not spawn a child. A `go build` without the tag (dev and `go test`) links the client into the server binary. A client, its goroutines, and any broker connection are created only inside `Dispatcher.Register`, and only when `STORAGE_KAFKA_BROKERS` is non-empty. An empty value does not construct a client and does not start the bridge. A bridge restart begins a new idempotent producer session, the same as a server restart. If the child exits, the server logs it, marks Kafka degraded, leaves in-flight rows pending, and starts the child again with backoff. The child has no TCP listener; stdin and stdout are the only IPC, and its stderr is copied to the server log. The child is killed when the server exits. The bridge produces one record at a time; per-key order stays in the dispatcher. `GET /api/v1/events/delivery` reads a cached status. It does not dial the broker. The producer uses `acks=all` and franz-go's default idempotent producer (this code does not call `DisableIdempotentWrite`). The record key is `tenant/bucket/key`. The body is S3-notification-shaped JSON plus a `datasafe` block; webhook and NATS bodies are unchanged. SASL PLAIN without TLS refuses startup. The RabbitMQ stub stays removed. RabbitMQ is not a planned sink.

The dispatcher keeps per-object order for every sink, not only Kafka. It does not publish a later row for the same tenant, bucket, and non-empty object key to a sink while an earlier row still has that sink pending. One stuck key does not block other keys or other sinks. An empty object key is not ordered. When the earlier attempt is `dead` or `delivered`, later rows for that key proceed. A dead earlier row is a gap: it is not published.

Delivery is at-least-once. A lost leader lease during `Publish` can duplicate a delivery. Empty sink lists mark the row delivered immediately. Sink names are snapshotted onto the row at insert; a sink registered later does not inherit older rows.

## Consequences

- Single-node bolt, default config, no `STORAGE_NATS_URL`, no enabled webhook, no bucket notification: no broker connection and no outbox rows for admin mutations or S3 object writes.
- S3 SigV4 put, copy, delete (including delete markers), and multipart complete write outbox rows in the same metadata transaction when a sink or a matching bucket rule exists. The gateway no longer posts bucket notifications itself.
- Paths that cannot share one transaction are documented in [event-outbox.md](../../operations-guide/en/event-outbox.md).
- Metrics `datasafe_events_outbox_pending`, `datasafe_events_delivered_total`, `datasafe_events_failed_total`, `datasafe_events_dead_total`, `datasafe_events_delivery_lag_seconds`, `datasafe_events_dropped_total`, `datasafe_events_sink_degraded`, and `datasafe_events_bridge_restarts_total` are emitted by the dispatcher. The overview dashboard row Event delivery and `deploy/docker/prometheus/alerts.yml` are the operator view.
