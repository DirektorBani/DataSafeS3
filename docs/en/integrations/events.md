**[Русский](../../ru/integrations/events.md)** | English

# Event integrations

Webhooks, NATS, and Kafka are an **external event feed**. They tell another system that an object or an admin change happened. They are not the cluster interconnect.

Nodes do not use these sinks to replicate bytes, to elect a leader, or to talk to each other. Object copies go through gateway replication, site replication, or the shared disk. Metadata failover is Postgres streaming plus the leader lease. See [High availability](../../operations-guide/en/high-availability.md).

Delivery is at-least-once from a durable outbox. Per-object order is held per sink. A sink that exhausts its attempts is dead for that row (a dead-letter gap). Details: [event outbox](../../operations-guide/en/event-outbox.md).

A default install registers no sink, opens no broker connection, and does not write outbox rows.

## Webhook

Create a webhook in the console (**Settings → Webhooks**). The webhook sink is registered while at least one webhook is enabled. The POST body stays `{"event","timestamp","data"}`.

Transport errors, timeouts, HTTP 5xx, 408, and 429 are retried. Any other 4xx marks that sink dead on the row. Bucket notification URLs use the same dispatcher and keep the body `{"event","bucket","key","size","time"}`.

## NATS JetStream

Set `STORAGE_NATS_URL`. An empty URL does not register a sink and does not dial. The subject is `STORAGE_NATS_SUBJECT` (default `datasafe.events`). The body stays `{"event","payload"}`.

If the server has JetStream and a stream covers `STORAGE_NATS_SUBJECT.<event>`, publish waits for an acknowledgement. Otherwise the publish is core NATS. Core publish does not confirm that a subscriber received the message. A JetStream ack confirms the stream stored it, not that a consumer processed it. There is no extra environment variable.

## Kafka

Kafka is optional. Set `STORAGE_KAFKA_BROKERS` to a comma-separated `host:port` list when an external consumer should read the feed. Leave it empty and the process does not create a client, a child process, or a connection. Installer profile `kafka` is off unless you pass it.

The container image includes the static client `/usr/lib/datasafe/datasafe-kafka-bridge`. The server starts that program only when the broker list is non-empty. The sink does not create topics. The message is S3-notification JSON plus a `datasafe` object, not the webhook or NATS envelope.

Setup, SASL, and the record shape: [Kafka](kafka.md).

## Console

**Settings → Event delivery** lists webhook, NATS, and Kafka. Each card shows status, queue depth, delivered and failed counts (the process counters `datasafe_events_delivered_total` and `datasafe_events_failed_total`), and the last error. The sentence about `STORAGE_KAFKA_BROKERS` is shown only while Kafka is disabled.

`GET /api/v1/events/delivery` reads that cache. It does not ping the broker. `POST /api/v1/events/test` inserts one outbox row. Both are administrator-only.

Grafana row **Event delivery** and `deploy/docker/prometheus/alerts.yml` use the same counters. See [monitoring](../../operations-guide/en/monitoring.md).
