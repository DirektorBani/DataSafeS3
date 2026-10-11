**[Русский](../ru/event-outbox.md)** | English

# Durable event outbox

Admin notifications are written to the metadata store and delivered by one dispatcher per `storage-server` process. A restart or a down receiver no longer drops the event: the row stays `pending` until every registered sink succeeds or exhausts its attempts.

This is unreleased work on top of v1.4.1. `VERSION` is unchanged.

## What is stored

| Field | Meaning |
|-------|---------|
| `id` | ULID, time-sortable |
| `created_at` | Insert time |
| `event_type` | `ObjectCreated`, `ObjectDeleted`, `BucketCreated`, `BucketDeleted`, `UserCreated`, `MultipartCompleted`, `ShareCreated` |
| `bucket`, `key`, `version_id`, `tenant_id` | Scope of the change |
| `payload` | JSON object historically sent as webhook `data` / NATS `payload` |
| `attempts` | Per-sink `attempts`, `status`, `next_attempt_at`, `last_error` |
| `status` | `pending`, `delivered`, `dead`, or `dropped` |

Bolt uses buckets `event_outbox` and `event_outbox_due`. They are created when the database opens, including an existing v1.4.x file. Postgres uses table `event_outbox` (migration `020`) and indexes `(status, next_attempt_at)` and pending `(created_at, id)`. A v1.4.x binary ignores that table. The down migration drops it.

## Dispatcher

One loop per process, poll interval 1 second. It is not an environment variable.

- **Single-node** (bolt, or HA disabled): this process delivers.
- **HA** (`STORAGE_HA_ENABLED` and the Postgres metadata backend): only the process that holds the existing leader lease delivers and purges. Other processes update the pending gauge and leave rows alone. Bolt has no leader lock, so enabling HA without Postgres does not elect a leader and the process still delivers.
- **Backoff:** 1s, 2s, 4s, … capped at 5 minutes, with ±20% jitter, never below 100ms.
- **Dead letter:** a sink that fails `STORAGE_EVENTS_MAX_ATTEMPTS` times (default 20) is marked `dead` on that row. The row is `dead` when no sink is still pending and at least one is dead.
- **Retention:** `delivered` rows are deleted `STORAGE_EVENTS_RETENTION` after `delivered_at` (default 24h). `dead` rows are deleted `STORAGE_EVENTS_DEAD_RETENTION` after `dead_at` (default 168h). `dropped` rows use the dead retention, measured from `updated_at`.
- **Cap:** `STORAGE_EVENTS_MAX_PENDING` (default 100000). `0` means unlimited. When a new row would exceed the cap, the oldest pending rows are marked `dropped` with reason `pending cap exceeded`. The object or bucket write still commits. The process increments `datasafe_events_dropped_total{reason="pending_cap"}` and writes Activity action `events_outbox_dropped`.

A sink is registered only when it is configured. NATS is registered when `STORAGE_NATS_URL` is set. The webhook sink is registered when at least one webhook is enabled, and that set is refreshed when webhooks are created, updated, or deleted. Kafka is not registered in this change.

Sink names that apply to the event are copied onto the row at insert. `nats` is copied when that sink is registered. `webhook` is copied when an enabled webhook lists the event type or `*`. A sink configured later does not receive rows already queued, and it does not receive events that were never written. If the snapshot is empty, the metadata change does not insert an outbox row. Those events are not replayed.

## Environment variables

None are required. Invalid values keep the default.

| Variable | Default | Meaning |
|----------|---------|---------|
| `STORAGE_EVENTS_RETENTION` | `24h` | How long a delivered row is kept |
| `STORAGE_EVENTS_DEAD_RETENTION` | `168h` | How long a `dead` or `dropped` row is kept |
| `STORAGE_EVENTS_MAX_PENDING` | `100000` | Pending rows before the oldest are dropped. `0` disables the cap |
| `STORAGE_EVENTS_MAX_ATTEMPTS` | `20` | Failures per sink before `dead` |

Webhook URLs stay in the console. NATS stays `STORAGE_NATS_URL` and `STORAGE_NATS_SUBJECT` (default subject `datasafe.events`). An empty URL does not register the sink and does not open a connection. The NATS body is still `{"event","payload"}`. The webhook body is still `{"event","timestamp","data"}`, with `timestamp` taken from the outbox row.

Each webhook keeps one delivery-history row per outbox event. Retries update `attempts`, `last_attempt`, `status_code`, and `error` on that row. The console retry action posts again and updates the same row. A successful manual retry marks the webhook sink delivered on the outbox row.

## Where the row is atomic

Bolt and Postgres both commit the outbox row in the same transaction as the metadata mutation for:

- admin object put (plain and versioned)
- bucket create and bucket delete (metadata; the object-backend delete of a bucket follows the transaction)
- object delete, including a version delete marker
- multipart completion's object row (`ObjectCreated` and `MultipartCompleted`)
- admin user create
- share create
- restore-from-trash object put

These paths are not one transaction with the event:

- **Object bytes** are written to the backend before the metadata transaction. A crash between those steps can leave bytes without a row, or a row whose delivery describes a metadata commit that did not happen if the process dies before commit. That split existed before the outbox.
- **Soft-delete** copies the object into the trash bucket, writes the trash row, then deletes the original. The event commits only with that delete. `trash_id` is filled after the trash row exists. A crash before the delete leaves trash data and no `ObjectDeleted` event.
- **Multipart** deletes the upload record after the object-row transaction.
- **Bucket create** commits the event with the bucket row. Visibility and team updates that follow are separate writes.
- **Tenant member create** writes the user, membership, and groups, then inserts the event. If that insert fails, the user still exists and the failure is logged.
- **S3 SigV4** puts, copies, and deletes do not emit these events yet. They move onto this outbox in E03, together with bucket-notification webhooks (`EmitBucketNotification` / gateway `OnBucketNotification`). The outbox row and `EventSink` are the path those calls will use. E01 does not write them.

A metadata store that cannot share a transaction falls back to an insert after the metadata commit. Both shipping backends share the transaction for the methods above. A failed insert on that fallback is logged and does not fail the API call.

## Delivery guarantees

At-least-once, not exactly-once. Webhook delivery tries every matching hook; any transport error or non-2xx fails the sink attempt, and a later retry calls the hooks again. 4xx is retried the same way as 5xx until `dead`. Narrowing that (no retry on 4xx except 408/429) and NATS JetStream acknowledgement are a later step.

If the leader lease is lost while `Publish` is in flight, the next leader can deliver the same row. The publish is not held inside `FOR UPDATE`.

With no NATS URL and no enabled webhook, admin mutations do not write outbox rows and do not increment `datasafe_events_delivered_total`. Turning a sink on later does not backfill those events.

## Metrics

The dispatcher updates these. A Grafana panel is not part of this change.

| Metric | Meaning |
|--------|---------|
| `datasafe_events_outbox_pending` | Rows in `pending` |
| `datasafe_events_delivered_total{sink}` | Successful sink deliveries |
| `datasafe_events_failed_total{sink,reason}` | Failed attempts (`reason="publish"`) |
| `datasafe_events_dead_total{sink}` | Sinks that exhausted retries |
| `datasafe_events_delivery_lag_seconds{sink}` | Insert to successful delivery |
| `datasafe_events_dropped_total{reason}` | Cap overflow (`reason="pending_cap"`) |

## Single-node default

Bolt, no Postgres, no `STORAGE_NATS_URL`, no enabled webhooks: the process does not open a broker connection and does not write outbox rows for admin mutations. S3 API behaviour is unchanged. An admin mutation writes a row only after a sink is configured for that event type.

## Decision for E03

S3 SigV4 object events and bucket-notification webhooks will move onto this outbox in E03. That is required for the Kafka use case. E01 leaves the dispatcher, the row schema, and `EventSink.Register` ready for those call sites and does not invent a second delivery path.
