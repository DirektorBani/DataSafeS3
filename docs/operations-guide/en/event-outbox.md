**[Русский](../ru/event-outbox.md)** | English

# Durable event outbox

Admin notifications and S3 SigV4 object events are written to the metadata store and delivered by one dispatcher per `storage-server` process. A restart or a down receiver no longer drops the event: the row stays `pending` until every registered sink succeeds or exhausts its attempts.

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

A sink is registered only when it is configured. NATS is registered when `STORAGE_NATS_URL` is set. The webhook sink is registered when at least one webhook is enabled, and that set is refreshed when webhooks are created, updated, or deleted. The bucket-notification sink is registered when a bucket has a notification URL (including after process start) and when a matching object write enqueues one. Kafka is registered when `STORAGE_KAFKA_BROKERS` is non-empty. An empty value does not create a client or a connection. See [Kafka](../../en/integrations/kafka.md).

## Per-object order

For each sink, the dispatcher does not deliver a later row for the same tenant, bucket, and object key while an earlier row still has that sink `pending`. List order is by due time, so a retry can make a delete due before its create. The sink still receives the create first.

One stuck key blocks only that key on that sink. Other keys keep moving. Other sinks on the same key are independent: a failing Kafka attempt does not hold the webhook attempt.

An empty object key is not held behind another empty key. `UserCreated` and bucket events use an empty key; ordering them would block every such event for the tenant.

When the earlier attempt becomes `dead` (attempts exhausted, or a terminal error), later rows for that key proceed. The dead row is not published. Consumers see a gap. They must not assume every change of that object was delivered.

Delivery stays at-least-once. A duplicate of an earlier `event_id` can still appear. Order is among distinct rows, per sink, per object key.

Sink names that apply to the event are copied onto the row at insert. `nats` is copied when that sink is registered. `webhook` is copied when an enabled webhook lists the event type or `*`. The bucket-notification sink is not copied onto those rows; a matching bucket rule gets its own row. A sink configured later does not receive rows already queued, and it does not receive events that were never written. If the snapshot is empty and no bucket rule matches, the metadata change does not insert an outbox row. Those events are not replayed.

## Environment variables

None are required. Invalid values keep the default.

| Variable | Default | Meaning |
|----------|---------|---------|
| `STORAGE_EVENTS_RETENTION` | `24h` | How long a delivered row is kept |
| `STORAGE_EVENTS_DEAD_RETENTION` | `168h` | How long a `dead` or `dropped` row is kept |
| `STORAGE_EVENTS_MAX_PENDING` | `100000` | Pending rows before the oldest are dropped. `0` disables the cap |
| `STORAGE_EVENTS_MAX_ATTEMPTS` | `20` | Failures per sink before `dead` |

Webhook URLs stay in the console. NATS stays `STORAGE_NATS_URL` and `STORAGE_NATS_SUBJECT` (default subject `datasafe.events`). An empty URL does not register the sink and does not open a connection. The NATS body is still `{"event","payload"}`. The webhook body is still `{"event","timestamp","data"}`, with `timestamp` taken from the outbox row.

On connect, NATS checks whether JetStream is enabled and whether a stream covers `STORAGE_NATS_SUBJECT.<event type>` (for example `datasafe.events.ObjectCreated`). If both are true, publish waits for a JetStream acknowledgement and a failed ack is retried. Otherwise the publish is core NATS. That choice is cached until the client reconnects, so a stream created later is picked up on the next connection. Core publish confirms only that the server accepted the message. A JetStream ack confirms the stream stored it. Neither confirms that a consumer processed it. There is no separate JetStream configuration variable.

Each console webhook keeps one delivery-history row per outbox event. Retries update `attempts`, `last_attempt`, `status_code`, and `error` on that row. The console retry action posts again and updates the same row. A successful manual retry marks the webhook sink delivered on the outbox row. Per-bucket notification URLs do not write a console delivery-history row.

Transport errors, timeouts, HTTP 5xx, 408, and 429 are retried. `Retry-After` (delta-seconds or HTTP-date) is honoured when it is longer than the normal backoff, and the wait is still capped at 5 minutes. Any other 4xx, and 3xx, marks that sink `dead` on the row immediately. `last_error` contains the status line. The delivery-history row is updated on both paths.

## Where the row is atomic

Bolt and Postgres both commit the outbox row in the same transaction as the metadata mutation for:

- admin object put (plain and versioned)
- S3 SigV4 put, copy, and delete, including a version delete marker
- bucket create and bucket delete (metadata; the object-backend delete of a bucket follows the transaction)
- object delete, including a version delete marker
- multipart completion's object row (`ObjectCreated` and `MultipartCompleted`, and a bucket-notification row when the rule matches)
- admin user create
- share create
- restore-from-trash object put

These paths are not one transaction with the event:

- **Object bytes** are written to the backend before the metadata transaction. A crash between those steps can leave bytes without a row, or a row whose delivery describes a metadata commit that did not happen if the process dies before commit. That split existed before the outbox.
- **Soft-delete** copies the object into the trash bucket, writes the trash row, then deletes the original. The event commits only with that delete. `trash_id` is filled after the trash row exists. A crash before the delete leaves trash data and no `ObjectDeleted` event.
- **Multipart** deletes the upload record after the object-row transaction. Completing a multipart upload does not enqueue object or site replication; that hook was not called before either.
- **Bucket create** commits the event with the bucket row. Visibility and team updates that follow are separate writes.
- **Tenant member create** writes the user, membership, and groups, then inserts the event. If that insert fails, the user still exists and the failure is logged.
- **S3 SigV4** object bytes are still written before the metadata transaction. The outbox row commits with the object or delete-marker row. Internal callers (trash copy, inventory, lifecycle) do not set the S3 flag, so they do not write `ObjectCreated` / `ObjectDeleted` unless an admin handler already built that row. They still enqueue a bucket notification when the bucket rule matches, which is what the old HTTP path did.
- **Bucket notification** is a second row, sink `bucket-webhook`, not a copy of the console webhook row. The POST body stays `{"event","bucket","key","size","time"}` with `Content-Type` and `X-DataSafe-Event`. `event` is still `put` or `delete` (`multipart` posts `put`). `bucket` is still the storage key. `time` is the object record time at commit, not the clock at HTTP send. The stored payload also has `_webhook_url`; that field is removed before the POST. A failed bucket URL is retried with the same 4xx / 429 rules. The old path discarded HTTP errors.

A metadata store that cannot share a transaction falls back to an insert after the metadata commit. Both shipping backends share the transaction for the methods above. A failed insert on that fallback is logged and does not fail the API call.

## Delivery guarantees

At-least-once, not exactly-once. The console webhook sink tries every matching hook on one attempt. A transport error, timeout, 5xx, 408, or 429 fails that attempt and a later retry calls the hooks again, including hooks that already returned 2xx. The sink is marked `dead` immediately only when every failure on the attempt is a non-retryable status. One hook's 400 does not by itself stop a sibling hook's 429 from being retried.

If the leader lease is lost while `Publish` is in flight, the next leader can deliver the same row. The publish is not held inside `FOR UPDATE`.

With no NATS URL, no enabled webhook, and no matching bucket notification, admin mutations and S3 puts do not write outbox rows and do not increment `datasafe_events_delivered_total`. Turning a sink on later does not backfill those events. Deleting every bucket notification and restarting leaves queued bucket posts without a sink; they retry, then go `dead`. During one process the sink stays registered after the first notification, so those posts are still attempted.

## Metrics

The dispatcher updates these. A Grafana panel is not part of this change.

| Metric | Meaning |
|--------|---------|
| `datasafe_events_outbox_pending` | Rows in `pending` |
| `datasafe_events_delivered_total{sink}` | Successful sink deliveries |
| `datasafe_events_failed_total{sink,reason}` | Failed attempts (`reason="publish"`, or `reason="terminal"` for a non-retryable status) |
| `datasafe_events_dead_total{sink}` | Sinks marked `dead` (attempt limit, or a non-retryable HTTP status) |
| `datasafe_events_delivery_lag_seconds{sink}` | Insert to successful delivery |
| `datasafe_events_dropped_total{reason}` | Cap overflow (`reason="pending_cap"`) |

## Single-node default

Bolt, no Postgres, no `STORAGE_NATS_URL`, no enabled webhooks, no bucket notification URL: the process does not open a broker connection and does not write outbox rows for admin mutations or for S3 puts, copies, deletes, or multipart completion. An object write grows the outbox only when a sink is configured for that event or the bucket rule matches the key.

## Bucket notification filters

`PutBucketNotificationConfiguration` still stores one webhook URL: the last non-empty topic, then the last non-empty queue, wins. Event names from every destination are concatenated. That collapse is unchanged.

`Prefix` and `Suffix` are optional (`Filter` / `S3Key` / `FilterRule`, and `Key` is accepted on input). An empty side matches every key. The filter is applied when the row is inserted, so a key that does not match is not stored and is not delivered later.

`s3:ObjectCreated:*` matches put, copy, and multipart completion. `s3:ObjectCreated:Put`, `Post`, and `Copy` match put and copy only. Copy is stored as `put`, so a Copy-only rule also matches a normal PUT, and a Put-only rule also matches a copy. `s3:ObjectCreated:CompleteMultipartUpload` matches multipart completion only. `s3:ObjectRemoved:*`, `s3:ObjectRemoved:Delete`, and `s3:ObjectRemoved:DeleteMarkerCreated` all match both a hard delete and a delete marker. The posted `event` value does not distinguish those cases.

`s3:ObjectCreated:*` did not used to match the posted event `put`, because a generic `:*` prefix check ran first and never reached that alias. It matches now.

## S3 events on the outbox

S3 SigV4 object events and bucket-notification webhooks are delivered from this outbox. That is the path a later Kafka sink registers on. There is still one dispatcher and one row schema.
