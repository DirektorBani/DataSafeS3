# ADR 0002: EventSink interface and durable outbox

**Status:** Accepted  
**Date:** 2026-07-10 (proposed) · 2026-10-11 (accepted)  
**Related:** E01 durable outbox · E02 Kafka sink (later) · E03 webhook/NATS retry classification (later)

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

Webhook and NATS are adapters on this dispatcher. A sink is registered only when configured: NATS when `STORAGE_NATS_URL` is set, webhook when at least one webhook is enabled (refreshed on webhook changes). An event whose snapshot is empty is not stored and is not replayed. Webhook retries every non-2xx until dead; classifying 4xx (except 408/429) as terminal is E03. NATS remains core publish; JetStream ack is E03. E03 also moves S3 SigV4 object events and bucket-notification webhooks onto this outbox; that is required for Kafka. The row and `Dispatcher.Register` are the extension point.

Kafka is not in this decision's runtime. E02 registers a sink with `Dispatcher.Register` only when `STORAGE_KAFKA_BROKERS` is set, and must not add a client or a connection when that variable is empty. **Do not** add a Kafka module to `go.mod` until E02. The RabbitMQ stub is removed. RabbitMQ is not a planned sink.

Delivery is at-least-once. A lost leader lease during `Publish` can duplicate a delivery. Empty sink lists mark the row delivered immediately. Sink names are snapshotted onto the row at insert; a sink registered later does not inherit older rows.

## Consequences

- Single-node bolt, default config, no `STORAGE_NATS_URL`, no enabled webhook: no broker connection and no outbox rows for admin mutations.
- S3 SigV4 handlers do not emit outbox rows. Per-bucket notification webhooks and the gateway `OnBucketNotification` path stay off the outbox.
- Paths that cannot share one transaction are documented in [event-outbox.md](../../operations-guide/en/event-outbox.md).
- Metrics `datasafe_events_outbox_pending`, `datasafe_events_delivered_total`, `datasafe_events_failed_total`, `datasafe_events_dead_total`, `datasafe_events_delivery_lag_seconds`, and `datasafe_events_dropped_total` are emitted by the dispatcher. A Grafana panel is E05.
