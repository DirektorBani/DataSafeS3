**[Русский](../../ru/integrations/kafka.md)** | English

# Kafka

Kafka is an opt-in event sink. A default install does not set `STORAGE_KAFKA_BROKERS`, does not start a broker, and does not open a connection. `VERSION` is still 1.4.1. This page is unreleased work.

The client is [franz-go](https://github.com/twmb/franz-go) (`github.com/twmb/franz-go`), pure Go, no CGO. That choice is recorded in [ADR-0002](../../architecture/adr/0002-event-sink-interface.md). The container image keeps the client in a second static binary, `/usr/lib/datasafe/datasafe-kafka-bridge`. The server process does not import the module. It starts that program only when `STORAGE_KAFKA_BROKERS` is non-empty, so a default process does not map the client and does not start a child. A `go build` without `-tags datasafe_sinkexec` links the client into the server binary instead; `go test` uses that path. Either way, the client object, its goroutines, and any dial exist only when the broker list is non-empty.

## Setup

Leave the variable empty to disable the sink.

| Variable | Default | Meaning |
|----------|---------|---------|
| `STORAGE_KAFKA_BROKERS` | empty | Comma-separated `host:port`. Empty disables the sink |
| `STORAGE_KAFKA_TOPIC` | `datasafe.events` | Produce topic |
| `STORAGE_KAFKA_CLIENT_ID` | `datasafe` | Kafka client id |
| `STORAGE_KAFKA_TLS` | false | Dial with TLS. Also on when a CA, cert, or key file is set |
| `STORAGE_KAFKA_TLS_CA_FILE` | | PEM CA bundle |
| `STORAGE_KAFKA_TLS_CERT_FILE` | | Client certificate. Set together with the key |
| `STORAGE_KAFKA_TLS_KEY_FILE` | | Client private key |
| `STORAGE_KAFKA_SASL_MECHANISM` | | `PLAIN`, `SCRAM-SHA-256`, or `SCRAM-SHA-512` |
| `STORAGE_KAFKA_SASL_USERNAME` | | SASL username. `STORAGE_KAFKA_SASL_USERNAME_FILE` overrides it |
| `STORAGE_KAFKA_SASL_PASSWORD` | | SASL password. `STORAGE_KAFKA_SASL_PASSWORD_FILE` overrides it |

`PLAIN` without TLS refuses to start. The error names `STORAGE_KAFKA_SASL_MECHANISM=PLAIN` and tells you to set `STORAGE_KAFKA_TLS=true` or provide CA/cert/key files. SCRAM is allowed without TLS; the password is not sent as plaintext, but the connection is still unencrypted. Use TLS for SCRAM in production.

A `*_FILE` value is read from disk and a trailing newline is stripped. A missing file fails startup. The file contents are not written to Activity.

Producer settings that are not environment variables:

- `acks=all`
- idempotent produce (franz-go default; this process does not disable it)
- record key `tenant/bucket/key` (slashes inside the object key are kept; read the JSON to recover the key)
- partitioner: sticky key, so one object key stays on one partition

Lab broker, not part of the default installer profiles:

```bash
./install.sh --yes --profiles core,kafka
```

That applies `deploy/compose/docker-compose.kafka.yml`: one KRaft broker, image `apache/kafka:3.9.2`, in-stack bootstrap `kafka:19092`, host port `localhost:9092`. The overlay sets `STORAGE_KAFKA_BROKERS=kafka:19092` on `storage-server`. A `kafka-init` container runs `kafka-topics.sh --create --if-not-exists --topic datasafe.events` after the broker accepts connections. The broker has `auto.create.topics.enable=false`. Windows: `.\install.ps1 -Yes -Profiles core,kafka`.

The sink never creates a topic. There is no `STORAGE_KAFKA_AUTO_CREATE_TOPIC`. Create `STORAGE_KAFKA_TOPIC` before the first event. While the topic is missing, produce keeps failing and the outbox retries. The error on **Settings → Event delivery** and in `GET /api/v1/events/delivery` (`sinks[].last_error`) is `topic does not exist`, followed by the broker text `UNKNOWN_TOPIC_OR_PARTITION`.

A change of `STORAGE_KAFKA_BROKERS` between process starts is Activity action `kafka_brokers_changed`. The resource name is the new broker list. SASL secrets are not logged.

## Message format

Kafka does not reuse the webhook or NATS body. Those stay as they are.

```json
{
  "Records": [
    {
      "eventVersion": "2.2",
      "eventSource": "datasafe:s3",
      "eventName": "s3:ObjectCreated:Put",
      "eventTime": "2026-10-11T12:00:00.000Z",
      "s3": {
        "bucket": { "name": "photos" },
        "object": {
          "key": "a/b.jpg",
          "size": 12,
          "eTag": "",
          "versionId": ""
        }
      }
    }
  ],
  "datasafe": {
    "tenant_id": "",
    "event_id": "01H...",
    "schema_version": 1
  }
}
```

| Stored `event_type` | `eventName` |
|---------------------|-------------|
| `ObjectCreated` | `s3:ObjectCreated:Put` |
| `ObjectDeleted` | `s3:ObjectRemoved:Delete` |
| `MultipartCompleted` | `s3:ObjectCreated:CompleteMultipartUpload` |
| `BucketCreated` | `s3:BucketCreated` |
| `BucketDeleted` | `s3:BucketRemoved` |
| `UserCreated` | `datasafe:UserCreated` |
| `ShareCreated` | `datasafe:ShareCreated` |
| `TestEvent` | `datasafe:TestEvent` |

`ObjectCreated` covers a copy as well as a put. The stored type does not distinguish them. `key` is the raw object key, not the URL-encoded form AWS notifications use. `size` and `eTag` are taken from the stored payload when those fields exist; otherwise size is `0` and `eTag` is empty. `versionId` is the outbox version id. `schema_version` is `1`.

## Delivery

Delivery is at-least-once. A leader change during publish, or a process restart after the broker accepted a record and before the outbox row was saved, can deliver the same `event_id` again. Idempotent produce removes duplicates from retries inside one client session. It does not remove duplicates across a restart. Consumers should treat `datasafe.event_id` as the idempotency key.

Order is per sink and per object key. The dispatcher does not publish a later row for the same tenant, bucket, and key to Kafka while an earlier row is still pending for the Kafka sink. One stuck key does not block other keys, and it does not block webhook or NATS for that key. An empty object key is not ordered (`UserCreated`, bucket create/delete).

If the earlier row's Kafka attempt goes `dead` (attempts exhausted, or a terminal error), later rows for that key are published. The dead row is not. That is a gap. Do not assume every change of the object is in the topic.

`GET /api/v1/events/delivery` (administrator) returns `connected`, `degraded`, or `disabled`, the last error, and queue depth. For Kafka, `connected` means the sink is registered, the last publish did not fail, and a ping on that request succeeded. It is not consumer lag. `POST /api/v1/events/test` queues one `TestEvent` for each registered sink. The console page is **Settings → Event delivery**.
