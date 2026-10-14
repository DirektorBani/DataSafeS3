# Changelog

All notable changes to DataSafeS3 are documented in this file.

## [1.6.0]

### Added

- **`storage-cli migrate from-minio`.** Copies buckets from a MinIO-compatible endpoint: versions oldest-first, delete markers, Object Lock, retention, legal hold, tags, user metadata, content-type, prefix lifecycle expiration, and bucket policy statements this server can evaluate. Resume state is JSONL (`--state`, default `./.minio-migrate-state`). A repeated run does not write the same source version twice. HTTP 503 `not cluster leader` is retried; several `--target` values pick the leader from `GET /healthz`. The report is `summary.md`, `evidence.json` (`verdicts` PASS/FAIL), and `evidence.zip`. **Settings → Migrate** prints the command and opens a local evidence file. Source credentials are not sent to the console. IAM import is not included. Guides: [EN](docs/operations-guide/en/migrate-from-minio.md) · [RU](docs/operations-guide/ru/migrate-from-minio.md). ADR [0007](docs/architecture/adr/0007-minio-s3-copier.md).

- **Vault Transit for SSE-KMS (opt-in).** The HTTP client starts only when `STORAGE_KMS_PROVIDER=vault-transit` and an address are set (environment or **Settings → Vault (KMS)**). Empty values open no connection, start no poller, and do not register KMS metrics. There is no Vault SDK. SSE-S3 and plaintext buckets do not call Vault. Until Vault is configured, `aws:kms` still wraps with the local server key. A cache miss while Vault is down fails the SSE-KMS read or write with HTTP 503 `KMS unavailable` and does not store a partial object. A cached data key still decrypts until `STORAGE_KMS_DEK_CACHE_TTL` (default 5m). Nodes drop cache entries when `kms_keys.generation` changes (`STORAGE_KMS_POLL_INTERVAL`, default 2s). The secret id and token are sealed in metadata even when field encryption is off, and are not returned by the settings API, `/healthz`, or Activity. The console page tests the connection, masks a saved secret, and lists each node's Vault status. A share of an SSE-KMS object while Vault is down returns `Файл временно недоступен, попробуйте позже`. Alert `DataSafeKMSUnavailable` fires when `datasafe_kms_available` stays 0 for 1 minute; that series exists only while Vault is enabled. Postgres migration `026`. Guides: [EN](docs/operations-guide/en/kms-vault.md) · [RU](docs/operations-guide/ru/kms-vault.md).

### Fixed

- **MinIO migration acceptance.** SigV4 rejects an `x-amz-*` header that is not in `SignedHeaders` (HTTP 400) and does not store that metadata. `Content-Type` may be unsigned, which is what minio-go and a browser presigned PUT send. Lifecycle `Filter/Prefix` is stored; a transition, noncurrent, tag, or date rule is rejected instead of becoming a whole-bucket expiration. Delete markers stay in chronological order. Object retention keeps COMPLIANCE on the object when the bucket default is GOVERNANCE. `ListObjects`, `ListObjectsV2`, and version listing page at `max-keys` (capped at 1000) with a continuation token. The copier lists each target bucket once, and a follower forwards writes to the leader address from the node heartbeat (`STORAGE_ADVERTISE_URL`, otherwise the hostname and port). The promote-drill and feature-audit clients sign `Content-Type` on a body, which the HTTP library adds even for an empty CreateBucket. An ETag that differs, including a multipart ETag and an SSE-KMS ETag that is not the plaintext MD5, is checked with SHA-256. Source bucket encryption is not copied, and an empty source configuration does not turn off encryption the target already has. `--verify-sample` checks at least one object, secrets can come from the environment or a file, and the settings page shows a PowerShell command and per-bucket counts.
- **Stock aws-cli 2 uploads.** CRC64NVME uses the reflected polynomial `0x9A6C9329AC4BC9B5` (`crc64.MakeTable`). The check value of `123456789` is `ae8b14860a799888`. Trailers for CRC32, CRC32C, SHA-1, SHA-256, and CRC64NVME are checked. CI runs `s3 cp` of a small object, a 20MiB multipart object, and `s3 sync` with aws-cli 2 defaults and no checksum flags.
- **CreateBucket keeps a bucket the caller already owns.** The response is HTTP 409 `BucketAlreadyOwnedByYou`. Encryption, versioning, and lock configuration are not replaced. The migration `encryption` verdict fails when a target bucket that had default encryption loses it, or a copied object is stored without `x-amz-server-side-encryption`.
- **Multipart verify on Postgres.** `CreateMultipartUpload` user metadata and content type are stored (migration `028`). A multipart ETag that does not match is checked by downloading the object and comparing SHA-256, including when `x-amz-meta-datasafe-source-version` is absent.
- **ListMultipartUploads and Object Lock versioning.** `GET ?uploads` lists an in-progress upload, including when the bucket storage key is not the logical name. A bucket with Object Lock reports versioning `Enabled`. Suspending that versioning is HTTP 409 `InvalidBucketState`.
- **Single-node gate baseline.** Bolt idle RSS and startup are medians of interleaved runs of this build against `ghcr.io/direktorbani/datasafe-storage-server:v1.5.0` on the same runner. Postgres is measured the same way, against that v1.5.0 image with `STORAGE_METADATA_BACKEND=postgres`, on a fresh database each run. Each comparison fails above baseline +10% RSS or baseline +1s startup.
- **aws-chunked bodies are stored decoded.** `STREAMING-AWS4-HMAC-SHA256-PAYLOAD`, `STREAMING-AWS4-HMAC-SHA256-PAYLOAD-TRAILER`, and `STREAMING-UNSIGNED-PAYLOAD-TRAILER` are unwrapped before the object or part is written. Chunk signatures are checked. `x-amz-decoded-content-length` is the stored size. A trailing checksum that does not match is HTTP 400 `BadDigest`. `GET ?location` returns the bucket region. A client that reads it before PUT, including mc, no longer treats the object listing as a region.
- **Object Lock on Postgres.** Retention mode is stored without a NUL in the jsonb document, so a lock, retention, or legal hold no longer fails with `unsupported Unicode escape sequence` (SQLSTATE 22P05). The newest row in `ListObjectVersions` is `IsLatest`, including a delete marker. Rewriting an older version for retention does not take that flag away.

- **Vault secret id and token are sealed and redacted.** `GET /api/v1/settings/system` no longer returns `kms.secret_id` or `kms.token`. Those values are AES-GCM (`kse:v1`) under the server key, or the JWT secret when `STORAGE_SSE_MASTER_KEY` is empty, even when `STORAGE_FIELD_ENCRYPTION_ENABLED` is false. Field encryption still stores them as `enc:v1`. Settings, Activity export, `/healthz`, and a metadata backup do not contain the plaintext. `PUT /api/v1/settings/system` cannot replace them.
- **A Postgres follower loads a Vault save within 2 seconds.** Shared settings are reread on the 2 second poll, not only on the 30 second heartbeat. A Bolt node does not run that poll. Until the provider and address are set, the poll does not open a client or register KMS metrics.
- **Save rejects Vault credentials that cannot encrypt.** `PUT /api/v1/kms/settings` probes encrypt and decrypt first and returns HTTP 400, or 408 on timeout, without storing the body. The console **Save** button stays disabled until **Test connection** succeeds for the current form. The probe deletes the Transit key `datasafe-connection-test`.
- **Console download and `/healthz` follow Vault.** An object download from the console returns HTTP 503 `KMS unavailable` when Vault is down. `kms.available` follows the latest probe, so a recovered node does not stay false. Standby health codes 429, 472, and 473 still count as up when login works.
- **A Vault token that cannot be renewed still connects.** `vault server -dev` answers `renew-self` for its root token with HTTP 400 `token not renewable`. That token is kept. HTTP 403, and any other HTTP 400, still reject it.

- **Plaintext objects survive adding `STORAGE_SSE_MASTER_KEY`.** 1.6.0 no longer marks every existing object as SSE-S3 format 0 just because a master key is set and the bucket is not empty. Unmarked objects are returned as stored. A format-0 reading of those bytes is opt-in (`STORAGE_SSE_LEGACY_FORMAT0=true`). A legacy flag written by the first 1.6.0 builds, which inferred it from the object count, is cleared on the next start unless that variable is set. The key fingerprint is unchanged: a different key still refuses to start.
- **Idle process does not start KMS.** With no `STORAGE_SSE_MASTER_KEY` on Bolt, startup does not create `kms_state` and does not run the cluster heartbeat. A key that was set earlier and then cleared still refuses to start. The error says the key is missing and to set `STORAGE_SSE_MASTER_KEY` to the original key. A different key still reports `SSE master key fingerprint mismatch`. The heartbeat a node does write is at least 1.6.0, so an image stamped from the 1.5.0 `VERSION` file does not treat itself as a mixed-cluster peer. Postgres and HA nodes write that heartbeat even when the key is empty, so a live 1.5.0 peer can be told apart from them.
- **Encryption stays off while a detected 1.5.0 node is live.** 1.5.0 does not write `cluster_node_heartbeats`. HTTP 409 `cluster_upgrade` is guaranteed for an unexpired `ha_leader_lock` whose holder has no 1.6.0 heartbeat, and for a cluster-registry node whose `GET /healthz` version is below 1.6.0. A 1.5.0 follower that is not in that registry leaves no version: its lease retry does not change the row, and `pg_stat_activity` has an empty `application_name`. On Postgres or HA the console requires the checkbox “All nodes are upgraded to 1.6.0” before enabling. S3 `PutBucketEncryption` has no checkbox. A 1.5.0 leader accepts `PutBucketEncryption` with its own code, so upgrade every node first.

## [1.5.0] - 2026-10-12

Minor release. Durable event delivery, optional Kafka, scripted HA promote, and per-rule gateway sync. `VERSION` is **1.5.0**. This commit does not push the git tag. Images are published when `release.yml` runs on `v1.5.0`.

### Known issues

- **SSE-S3 writes with `STORAGE_SSE_MASTER_KEY` fail.** `PutObject` compares the caller’s plaintext `Content-Length` with the ciphertext the backend just wrote (`internal/storage/fs.go`, and the same check in the erasure backend). The sizes differ by the GCM nonce and tag, so the write is rejected and no format-0 object is stored. Reads of objects written without the key are unaffected. 1.6.0 does not use that size check for a new encrypted object: the stored length is the plaintext length, and the body is the format-1 envelope. Leave the variable unset on 1.5.x unless you are reproducing this bug.

### Upgrade

- Postgres migrations `020` (`event_outbox`) and `021` (gateway sync schedule) run on startup. Bolt opens the outbox buckets when the database opens. A v1.4.x binary leaves them in place and ignores them. The down migrations drop the table and its events, or the schedule columns.
- The no-op Kafka and RabbitMQ sinks are removed. Kafka delivers only when `STORAGE_KAFKA_BROKERS` is set. An empty value does not dial. Webhook and NATS settings are unchanged. These sinks are an external event feed, not the cluster interconnect.
- Lab ports moved. Erasure defaults are console **8084**, S3 **9004**, Postgres **5444**. Site A defaults are console **8086**, S3 **9006**, Postgres **5446**. Delete an old `.env.erasure` or `.env.site-a` and copy the example again. A copied file keeps the previous ports.
- Automatic storage-leader takeover waits out the current lease. A 10s lease takes about 9s. The default 30s lease takes about 26s. A process that already held the lease and then lost it does not acquire it again.

Guides: [events EN](docs/en/integrations/events.md) · [RU](docs/ru/integrations/events.md) · [HA EN](docs/operations-guide/en/high-availability.md) · [RU](docs/operations-guide/ru/high-availability.md) · [upgrade EN](docs/operations-guide/en/upgrade.md#upgrading-to-v150) · [RU](docs/operations-guide/ru/upgrade.md#обновление-до-v150).

### Added

- **Gateway sync schedule (opt-in).** A replication rule with an empty `sync_schedule` still follows `STORAGE_GATEWAY_FULL_SYNC_INTERVAL` (default `1h`; `STORAGE_GATEWAY_CONTINUOUS_SYNC=true` shortens that global scan to 5 minutes). That scan queues objects and does not write a sync job. `off` stops the scheduled full scan for that rule; object events still replicate. A duration from `1m` to `168h` copies only that rule, writes a sync job, and stores the last run (`last_sync_at`, `last_sync_status`, `last_sync_error`, `last_sync_objects`). The console rule card shows the schedule, the last run, and the error. `PATCH /api/v1/gateway/replication/{id}` changes the schedule. Postgres migration `021`. This is not continuous multi-destination sync.

- **HA lab port split.** Erasure defaults are console **8084**, S3 **9004**, Postgres **5444**. Site A defaults are console **8086**, S3 **9006**, Postgres **5446**. Both overlays override the storage port, so they no longer publish `:9000`. Site A’s cluster endpoint defaults to its own S3 port. Vault stays **9001** / **5434** / **8200**. HA stays **8082** / **9002** / **5442**, with standby HTTP **19011** and **19013**. The HA NATS profile publishes **14222** and **18222**. `scripts/ha/*.ps1` read the stand `.env` when it exists. An older copied `.env.erasure` or `.env.site-a` keeps its previous ports until it is recreated from the example.

- **Promote drill without host Python.** `bash scripts/ha/run-promote-drill.sh` runs its helpers inside the webhook container. A static Docker client is downloaded once into the drill state directory and mounted there. Git Bash on Windows does not need Python installed. Linux state stays `/tmp/datasafe-promote-drill`. The SSH lab gate still uses host `python3`.

- **Scripted promote and a DR evidence pack.** `scripts/ha/promote.sh` and `scripts/ha/promote.ps1` fence the old Postgres primary, promote the standby, and on a fresh promote clear `ha_leader_lock`. A second run is safe and leaves a live lease in place. `scripts/cluster/promote.sh` and `.ps1` are the Patroni switchover with the same exit codes. Dry-run prints the plan and changes nothing. `scripts/collect-dr-evidence.sh` and `.ps1` write a folder, `summary.md`, `evidence.json`, and a zip (timing, lag, healthz, object write/read, outbox, dispatcher on the new leader, loss, at-least-once duplicates, versions). `bash scripts/ha/run-promote-drill.sh` is the one-command lab; CI job `ha-promote-drill` runs it. Host ports default away from `:9000` and `:9001`. This is scripted promote, not automatic multi-AZ failover. See [EN](docs/operations-guide/en/disaster-recovery.md) · [RU](docs/operations-guide/ru/disaster-recovery.md).

- **Event delivery observability.** The overview dashboard has an **Event delivery** row: outbox pending, delivered/failed/dead rates by destination, dropped rate by reason, delivery lag p50/p95/p99, the degraded gauge, and Kafka bridge restarts (`datasafe_events_bridge_restarts_total`, `datasafe_events_sink_degraded`). Example alerts are `deploy/docker/prometheus/alerts.yml` (pending above 1000 or growing for 10 minutes, any destination degraded for 5 minutes, dead deliveries increasing, pending-cap drops increasing). Compose loads that file. CI job `event-observability` runs `promtool check rules` and provisions Grafana `13.2.3` to read dashboard `datasafe-overview`. Activity writes `events_sink_degraded` on the first real failure of a destination and `events_sink_recovered` when it connects again; retries do not add rows. The console Activity filter lists those actions and `events_outbox_dropped`. Before the first successful probe, delivery status is `degraded` with `last_error` `waiting for first successful connection`. See [EN](docs/operations-guide/en/monitoring.md) · [RU](docs/operations-guide/ru/monitoring.md).

- **Kafka sink (opt-in).** `STORAGE_KAFKA_BROKERS` registers a [franz-go](https://github.com/twmb/franz-go) producer (pure Go, no CGO) with `acks=all`, the default idempotent producer, record key `tenant/bucket/key`, and topic `STORAGE_KAFKA_TOPIC` (default `datasafe.events`). TLS CA/cert/key files, SASL PLAIN (refused unless TLS is on), and SCRAM-SHA-256/512. Username and password accept `*_FILE`. An empty broker list does not create a client, a goroutine, or a connection. The body is S3-notification JSON (`Records[]`, `eventName`, `s3.bucket.name`, `s3.object.key/size/eTag/versionId`) plus `datasafe` (`tenant_id`, `event_id`, `schema_version: 1`). Webhook and NATS keep their envelopes. Put, copy, and multipart store `etag`, `size`, and the object version id from the committed object on the outbox row at insert. The dispatcher holds per-object order for every sink: a later row for the same tenant/bucket/key is not published to a sink while an earlier row is still pending on that sink. One stuck key blocks only that key. When the earlier attempt goes `dead`, later rows proceed and the dead row is a gap. Settings → Event delivery shows connected/degraded/disabled, last error, and queue depth; **Send test event** is administrator-only (`GET /api/v1/events/delivery`, `POST /api/v1/events/test`). A broker-list change between starts is Activity `kafka_brokers_changed`. Lab overlay `deploy/compose/docker-compose.kafka.yml` (`apache/kafka:3.9.2`) and installer profile `kafka` are not in the default profiles. See [EN](docs/en/integrations/kafka.md) · [RU](docs/ru/integrations/kafka.md).

- **Single-node regression gate.** CI job `single-node-gate` (pull requests into `main`, and pushes to `main`) builds the storage-server image and runs only that container the way installer profile `core` does: Bolt, `STORAGE_DEV=false`, no NATS or Kafka, Postgres hostname set but no Postgres process. Smoke covers `/healthz`, admin login and first-run setup, S3 put/get/list with the AWS CLI, Activity, inventory create/list/download, and an evidence-pack zip. It fails on broker dials in logs or outbound TCP, and when the outbox pending metric is not zero with no sinks. Idle RSS and startup-to-healthy are medians of five interleaved runs against `ghcr.io/direktorbani/datasafe-storage-server:v1.4.1` on the same runner; the job fails above baseline +10% RSS or baseline +1s startup. `./install.sh --yes --profiles core` runs the same smoke. `.\install.ps1 -Yes -Profiles core` is the Windows command; CI parses that script and does not boot Docker on Windows. See [EN](docs/operations-guide/en/single-node-gate.md) · [RU](docs/operations-guide/ru/single-node-gate.md).

- **Durable event outbox.** Admin notification events (object, bucket, user, share, multipart, trash) are stored in the metadata backend before delivery: bolt buckets `event_outbox` and `event_outbox_due`, or Postgres table `event_outbox` (migration `020`). One dispatcher per process delivers to webhook and NATS with exponential backoff and jitter, per-sink attempts, and `dead` after `STORAGE_EVENTS_MAX_ATTEMPTS`. Delivered rows expire after `STORAGE_EVENTS_RETENTION` (default `24h`); dead and cap-dropped rows after `STORAGE_EVENTS_DEAD_RETENTION` (default `168h`). `STORAGE_EVENTS_MAX_PENDING` (default `100000`) drops the oldest pending rows, increments `datasafe_events_dropped_total`, and writes Activity `events_outbox_dropped`. Object writes are not failed by that cap. In HA only the Postgres leader dispatches. None of the `STORAGE_EVENTS_*` variables are required. See [EN](docs/operations-guide/en/event-outbox.md) · [RU](docs/operations-guide/ru/event-outbox.md). ADR-0002 is Accepted.

### Changed

- **Event delivery page.** Each sink shows delivered and failed counts from `datasafe_events_delivered_total` and `datasafe_events_failed_total` (`GET /api/v1/events/delivery`). Those counters are process memory and reset to zero when storage-server restarts. The sentence that Kafka stays off until `STORAGE_KAFKA_BROKERS` is set is shown only while that sink is disabled. The console sidebar shows the server version from `GET /api/v1/health`.
- `GET /healthz` and `GET /api/v1/health` include `version` and `commit`, set with `-ldflags` from `VERSION` and the git commit in `deploy/docker/Dockerfile`. `release.yml` passes both when it publishes the tag.
- Webhook and NATS delivery goes through the outbox. `STORAGE_NATS_URL` and console webhook settings are unchanged. An empty NATS URL does not register a sink and does not dial. A webhook sink is registered only while at least one webhook is enabled. With no configured sink, admin mutations do not write outbox rows, and events from that period are not replayed later. Each console webhook keeps one delivery-history row per outbox event; retries update that row.
- Removed the no-op RabbitMQ and Kafka sinks from process wiring. There is one `events.EventSink` interface. Kafka is a real sink only when `STORAGE_KAFKA_BROKERS` is set. The container image runs franz-go in `/usr/lib/datasafe/datasafe-kafka-bridge` and starts it only then. A `go build` without `-tags datasafe_sinkexec` links it into the binary and still does not dial when brokers are empty.
- **S3 SigV4 object events** (put, copy, delete including delete markers, multipart complete) write outbox rows in the same metadata transaction when a sink is registered. With no sink and no matching bucket notification, those PUTs do not grow the outbox.
- **Bucket-notification webhooks** are delivered by the outbox dispatcher. Optional prefix/suffix filters apply when the row is inserted. The POST body stays `{"event","bucket","key","size","time"}`.
- **Webhook retry classification.** Transport errors, timeouts, HTTP 5xx, 408, and 429 are retried. `Retry-After` is honoured and capped at 5 minutes. Any other 4xx marks that sink dead immediately; `last_error` contains the status.
- **NATS JetStream.** If the connected server has JetStream and a stream covers `STORAGE_NATS_SUBJECT.<event>`, publish waits for an ack (failure is retried). Otherwise core publish is used. There is no extra environment variable.

### Fixed

- **Fresh install console.** Default Compose copies `/srv/console` from `DATASAFE_CONSOLE_IMAGE` (`ghcr.io/direktorbani/datasafe-console:v1.5.0`) into the `console-dist` volume and Caddy serves that volume. A `git archive` tree has no `web/console/dist`, so the old bind mount was an empty directory and `GET /` was 404 while `/healthz` still returned 200. `install.sh` and `install.ps1` now require `GET /` 200 and the `id="root"` marker on the edge (port `PUBLIC_HTTP_PORT`, default 8080). Host `web/console/dist` is the overlay `deploy/compose/docker-compose.local-console.yml`.
- **Simultaneous start on empty Postgres.** Several nodes no longer exit with `user already exists`. Bootstrap of the admin user runs under the migration advisory lock, and an existing admin is success.
- **Webhook delivery after HA takeover.** A follower reloads sinks from the database when it becomes leader and about every 2 seconds, so a webhook created after that process started is registered. An outbox row for a sink that is not registered yet stays pending and does not count attempts toward dead. Takeover waits out the current lease (about 9s for a 10s lease, about 26s for the default 30s lease).
- **Promote-drill Docker client on Git Bash.** The helper looked for an executable bit. Git Bash does not set that on the Linux client, so every run downloaded the 82 MB archive again. The script now keeps a non-empty file, writes the download to a partial tarball, and resumes with `curl -C - --retry 5 --retry-delay 3`, including through `HTTPS_PROXY`.
- **Gateway connection create response.** `POST /api/v1/gateway/connections` no longer returns `secret_key`. The list endpoint already omitted it. The stored credential is unchanged.
- **Concurrent PUT of one key.** Writers on one node no longer share `<key>.tmp` or insert a second unversioned row. Temps are unique, Bolt `Put` and Postgres `ON CONFLICT` keep the last writer, and that writer's bytes, etag, and size agree. Twenty parallel PUTs of one key all return 200.
- **Parallel Postgres migrate.** Nodes that start together on an empty database take a session advisory lock around migrations, so `CREATE EXTENSION` in `001_init` does not hit `pg_extension_name_index`.
- **Short HA leader lease.** A lease under 15 seconds used to renew on the same interval as the lease. The renew interval is now at most one third of the lease.
- **Follower lease takeover.** A storage node that has never held the lease keeps trying and takes it only after the previous lease expires. A process that already led and then failed renew still drops the lease immediately and does not acquire it again. This is not Postgres failover and not a power fence.
- **Gateway rule card.** The console shows the rule name when the rule has one.

- **Event bridge restarts during a broker outage.** A produce that outlasted the parent deadline used to kill the child and increment `datasafe_events_bridge_restarts_total`. The child now answers within its own deadline and stays up. The counter moves only when the process exits.
- **Dispatcher republish after a read-only primary.** A promote that fenced Postgres used to leave the old leader publishing the same outbox row on every tick until renew failed, because the webhook had already been sent and the delivered mark could not be written. A read-only or not-leader error from the store now stops the dispatcher and drops the lease. That process does not publish the row again. The new leader may deliver it once.
- **Single-node Prometheus scrape.** The shipped `cluster-nodes.json` target list is empty, so job `datasafe-cluster-nodes` does not scrape `storage-server:9000` a second time. Add extra nodes there when you have them.

- **Windows installer stderr.** `install.ps1` runs every `docker` / `docker compose` call through `Invoke-NativeCommand`. Windows PowerShell 5.1 with `$ErrorActionPreference = 'Stop'` was treating compose progress on stderr as a terminating error when the script output was redirected (`*> log`), so `-Yes -Profiles core` exited 1 while the stack was already up. The helper keeps `ErrorActionPreference` at Continue, sets `PSNativeCommandUseErrorActionPreference` to false on PowerShell 7.3+, returns stderr as text, and uses `$LASTEXITCODE` as the only success check. `-Yes` still waits for `http://127.0.0.1:8080/healthz` and exits 0 when that responds. CI on `windows-latest` runs that helper under redirected output in Windows PowerShell 5.1 and PowerShell 7. The Docker install itself stays on the Linux gate.
- Console Playwright `buckets.spec.ts` no longer matches both the bucket-table search and the command palette. In Russian both placeholders start with «Поиск бакетов». The spec now matches only `Search buckets...` / `Поиск бакетов...`. `gateway.spec.ts` accepts the English, Russian, and French page titles, and the Russian empty state «Подключения шлюза не настроены».

### Honesty

- Promote is a script. Nothing in this tree elects a new Postgres primary on its own, and nothing fails a node over between availability zones. A storage follower that shares the database retries the leader lease and takes it only after expiry. A process that already held the lease and then lost it does not try again. The fence is `default_transaction_read_only` on the old primary plus an immediate drop of the storage-server leader lease when renew fails. A superuser can turn that GUC off in a session. The script does not rebuild the old primary as a standby. Object bytes stay on the node that wrote them; the drill's post-promote read is a new object on the new leader. Delivery during the drill is at-least-once: one extra post is allowed for a row that was published and then could not be marked delivered. The compose drill runs the image from `deploy/docker/Dockerfile`. CI job `ha-promote-drill` loads the `docker-build` artifact instead of building that image a second time. Node-a is started and becomes leader before node-b starts. The SSH lab gate runs the promote dry-run and a healthz evidence pack. The timed Patroni step is still the existing kill-leader drill.
- Container images for this release are published when `release.yml` runs on the git tag `v1.5.0`. This commit does not push that tag. `GET /healthz` and `GET /api/v1/health` report `version` and `commit` from the image ldflags. The single-node gate still compares RSS and startup with the published `v1.4.1` image, because `v1.5.0` is not on GHCR until the tag is pushed. The installer step retags the pull-request server image and builds the console image as `v1.5.0` locally so Compose uses the pull-request bits. A fresh `git archive` plus `install.sh --yes --profiles core` must return `GET /` 200.
- `datasafe_events_sink_degraded` is 1 from registration until the first successful probe, so a destination that has not connected yet looks degraded. The 5-minute alert does not fire for a probe that succeeds immediately. Activity does not record the waiting sentence, only a real error and the later recovery. Dropped events have a reason label, not a destination label. Compose pins Grafana `13.2.3` and Prometheus `v3.15.0`. Helm Prometheus and Grafana image tags in `values.yaml` stay `v1.2.0`. The DataSafeS3 server and console tags in that file are `v1.5.0`. The Helm overview JSON includes the same Event delivery row, and Helm Prometheus does not mount the example alert file.
- The gate's resource limits can still fail on a noisy GitHub runner when most PR samples are slow. The step summary prints min and max beside the medians. The job does not widen the +10% or +1s limits.
- GHCR login in `single-node-gate` is optional. It is skipped for fork pull requests and when no token is present, and a failed login does not fail the job. `ghcr.io/direktorbani/datasafe-storage-server:v1.4.1` is a public image.
- The installer step retags the PR server image as `ghcr.io/direktorbani/datasafe-storage-server:v1.5.0` and tags a locally built `ghcr.io/direktorbani/datasafe-console:v1.5.0` so Compose uses the pull request bits. It does not push those tags. The resource baseline stays the published `v1.4.1` image. Outbox emptiness is the Prometheus gauge, not a Bolt key count. The TCP check does not see UDP DNS.
- Object bytes are written before the metadata transaction. The S3 outbox row commits with the object or delete-marker row. Soft-delete commits the event with the original-object delete, after the trash copy and trash row. Multipart completion deletes the upload record in a following transaction and does not enqueue replication. Creating a tenant member writes the user, membership, and groups first, then inserts the event. Bucket create is one transaction with the bucket row; visibility and team updates after that are separate.
- Bucket notification `time` is the object record time at commit, not the HTTP send time. `event` stays `put` or `delete` (multipart posts `put`). `bucket` stays the storage key. The stored payload includes `_webhook_url`, which is stripped before the POST. There is no console delivery-history row for a per-bucket URL. Copy and put share the event value `put`, so a Copy-only filter also matches a normal PUT. Delete and delete-marker share `delete`. `PutBucketNotificationConfiguration` still keeps one URL (last topic, then last queue). `s3:ObjectCreated:*` now matches the posted event `put`; previously the generic `:*` check prevented that.
- A bucket notification that fails is retried. The previous HTTP path discarded the error. 3xx is terminal, same as 4xx other than 408 and 429. The console webhook sink retries every matching hook together: one hook's 429 retries hooks that already returned 2xx. The sink is dead immediately only when every failure on that attempt is terminal.
- NATS core publish does not confirm subscriber delivery. A JetStream ack confirms the stream stored the message, not that a consumer processed it. The mode is cached until reconnect.
- Delivery is at-least-once. A second leader that appears while a publish is in flight can deliver the same row again. Followers do not dispatch. Bolt has no leader lock, so the single process always dispatches. Per-object order is per sink and per non-empty key. It does not stop a duplicate of an earlier `event_id`. An earlier row that goes `dead` is not published; later rows for that key are. Empty keys (`UserCreated`, bucket events) are not ordered against each other. Kafka consumers should dedupe by `event_id`. Idempotent produce covers retries inside one client session, not a process restart. Put, copy, and multipart copy `etag`, `size`, and `versionId` from the committed object into the outbox row at insert. `versionId` is empty when versioning is off. Webhook `data` and the NATS `payload` object include `etag` and `size` for those events; the envelopes stay `{"event","timestamp","data"}` and `{"event","payload"}`. Bucket-notification posts stay `{"event","bucket","key","size","time"}`. `ObjectCreated` is mapped to `s3:ObjectCreated:Put`, including copies, because the stored type does not distinguish copy. The object key in JSON is the raw key, not AWS URL-encoding. "Connected" on the delivery page is a cached status: the sink is registered, a probe or a publish has succeeded, and the last publish did not fail. `GET /api/v1/events/delivery` does not ping the broker. A background probe uses a 2s timeout, and publish failures update the same cache, so a down broker returns `degraded` with the dial or topic error in under a second. It is not a consumer-lag probe.
- The storage-server image does not link franz-go into `/storage-server`. The client is the static child `/usr/lib/datasafe/datasafe-kafka-bridge`, started only when `STORAGE_KAFKA_BROKERS` is set. The image builds the server with `-tags datasafe_sinkexec` (no CGO). The child inherits the server environment; it is not a separate security boundary. `go test` and a normal `go build` link the client in-process. If the child exits, the server logs `event bridge exited`, marks the sink degraded, leaves the rows pending, and restarts the child with backoff. The child speaks only on stdin and stdout; it has no TCP listener, and its stderr is copied to the server log. It exits when the server exits. Restarting the child starts a new idempotent producer session, the same as restarting the server. The child produces one record at a time. Produce and the status ping are bounded so a stopped broker cannot hold a request for minutes. Built here with the same Docker builder, the image is 19,719,481 bytes of compressed content versus 15,872,560 for a rebuild of `origin/main` (`530eec9`), a delta of +3,846,921 bytes (+24.2%). `docker images` shows 75.6MB versus 62.1MB. The published `ghcr.io/direktorbani/datasafe-storage-server:v1.4.1` image is 15,801,193 bytes (61.8MB). The child binary in the image is 9,511,074 bytes. The sink never creates topics. There is no `STORAGE_KAFKA_AUTO_CREATE_TOPIC`. A missing topic stays retriable and `last_error` is `topic does not exist` plus `UNKNOWN_TOPIC_OR_PARTITION`. The lab overlay creates `datasafe.events` with `kafka-topics.sh` and sets `auto.create.topics.enable=false`. The integration test creates its own topic the same way, through the Kafka protocol, and does not rely on broker auto-create.
- Default single-node still opens no broker connection. With no NATS URL, no enabled webhook, and no bucket notification it writes no outbox rows, including for S3 PUTs. Events from that period are not replayed. Deleting every bucket notification and restarting leaves queued bucket posts without a sink.
- Internal object writes (trash copy, inventory, lifecycle) do not emit global `ObjectCreated` / `ObjectDeleted` rows. They still enqueue a bucket notification when the bucket rule matches.

## [1.4.1] - 2026-10-11

Patch on **v1.4.0**. No new product features.

### Fixed

- **Release pins** — installer defaults, Compose/Helm/lab image tags, the console package version, and the current-release lines in the docs still pointed at v1.3.0 or older, so a README install pulled the previous release. They now track **v1.4.1**. `scripts/bump-release-version.sh` is the release-time bump; `scripts/check-release-pins.sh` fails CI, and `release.yml` fails the tag push, when those pins do not match `VERSION` (or the pushed tag).
- **Inventory create** — `POST /api/v1/inventory/jobs` for a bucket name that does not resolve returned **201** with status `failed` and error `not found`. It now returns **404** and does not store a job. The scan uses the same tenant- and owner-scoped resolution as the console and other admin routes, including when another tenant has the same logical name. An unknown `dest_bucket` is **404** as well.
- **Inventory list** — `GET /api/v1/inventory/jobs` was **405**. It is now an admin-only list, newest first, `limit` default 50 and maximum 100. OpenAPI full spec includes the operation.
- **Windows source builds** — there was no `.gitattributes`, so Git on Windows (`core.autocrlf=true`) checked `*.sh` out as CRLF. Building the server image then crash-looped with `exec /docker-entrypoint.sh: no such file or directory`. `*.sh` and the other files copied or bind-mounted into Linux containers are forced to LF. The server and lab Dockerfiles strip CR from entrypoints before `chmod`. The server image starts through `/usr/local/bin/datasafe-entrypoint`, which strips again so a bind-mounted `docker-entrypoint.sh` (HA local, local-binary) still runs. The HA object-replicator entrypoint and the Vault overlay do the same for their bind-mounted scripts.

### Changed

- **Default install is production-safe.** Compose and `.env.example` default `STORAGE_DEV` to false. The installer writes `STORAGE_DEV=false` unless `-Dev`, `--dev`, or profile `dev`. It generates `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY`, and `STORAGE_METRICS_TOKEN` when they are missing or still the published defaults, and the Compose Prometheus service scrapes with that bearer so Grafana keeps working. Profile `identity` sets `STORAGE_DEV=true` so the HTTP lab IdP still works. Dev and audit overlays set `STORAGE_DEV=true` explicitly. Upgrade notes: [EN](docs/operations-guide/en/upgrade.md#upgrading-to-v141) · [RU](docs/operations-guide/ru/upgrade.md#обновление-до-v141).
- Removed the one-shot workflow `.github/workflows/deferred-release-v1.2.0.yml` (v1.2.0 is already published).
- **Lab S3 image** — Docker Hub `minio/minio` no longer pulls (`pull access denied`). Feature-audit, `scripts/start-minio-test.cmd`, the Helm lab target, and the EN/RU gateway examples use `pgsty/silo:RELEASE.2026-09-16T00-00-00Z` (same `server /data` arguments and `/minio/health/live`). Anonymous `quay.io/minio/minio` pulls return 401, so that registry is not a substitute.

### Security

- **Go 1.26.9 and golang.org/x/text v0.41.0** — `govulncheck` reported GO-2026-6629 in `golang.org/x/text@v0.38.0` (fixed in v0.41.0) and net/http issues in the Go 1.26.8 standard library (GO-2026-6617, GO-2026-6613, GO-2026-6612, GO-2026-6611, GO-2026-6610, GO-2026-6608, and the rest of that set), fixed in Go 1.26.9. `go.mod` is `go 1.26.9` (that line selects toolchain go1.26.9; `go mod tidy` drops a redundant `toolchain` directive). `deploy/docker/Dockerfile` and CI `setup-go` (including feature-audit) pin **1.26.9**. Release images are built from that Dockerfile.

### Honesty

- An empty `STORAGE_METRICS_TOKEN` still leaves `/metrics` open (v1.1.0 legacy) and logs a warning when dev mode is off. The installer no longer leaves the token empty. A hand-written Compose file that omits it still has open metrics.
- The identity lab profile enables dev mode because that lab IdP is plain HTTP. It is not a production install.
- `GET /inventory/jobs` reads the in-process registry (lost on restart) and is capped at 100 rows. It is not a durable export history.
- Offline lab scripts still fall back to older local image tags when `v1.4.1` is not cached. The preferred default is `v1.4.1`.
- This patch does not publish container images. `ghcr.io/direktorbani/datasafe-storage-server:v1.4.1` and `datasafe-console:v1.4.1` appear when `release.yml` runs on the git tag.
- An existing Windows checkout keeps CRLF copies of `*.sh` until Git checks those files out again (`git checkout -- "*.sh"` after pulling). The image build and the entrypoint launcher still strip CR. Postgres init scripts such as `02-replication-hba.sh` are bind-mounted, so they need that refresh (or a fresh clone) before the first database init.

## [1.4.0] - 2026-08-24

### Added

- **Governance Evidence Pack (CE)** — Admin storage inventory CSV jobs (`POST/GET /api/v1/inventory/jobs`, download); Activity export CSV/JSON (`GET /api/v1/activity/export`); Object Lock / retention / blocked-delete activity codes; optional activity trail GC via `STORAGE_ACTIVITY_RETENTION_DAYS` (default 90).
- **S3 path audit** — SigV4 `DeleteObject` blocked by retention/legal hold now writes `object_delete_blocked` to Activity (same as Admin JSON delete).
- **Console** — Activity Export buttons; bucket Settings → Storage inventory CSV with optional dest-bucket (admin).
- **Helper** — `scripts/collect-evidence-pack.ps1` assembles inventory + activity + settings into a folder/zip.
- **Docs** — EN/RU [governance evidence](docs/use-cases/en/governance-evidence.md) checklist; audit guide updates.
- **Tests** — Go evidence-pack coverage (inventory, activity export, S3 delete-blocked, retention purge); Playwright smoke `e2e/evidence-pack.spec.ts`.

### Changed

- Activity retention worker runs once at process start, then hourly (was hourly-only).
- Inventory `schedule` other than `manual` returns **501** (cron not shipped).

### Honesty

- Inventory is Admin-first CSV for operator evidence — **not** AWS S3 Inventory + Athena / Parquet analytics; cron/durable queue deferred.
- Activity retention GC is disk hygiene — **not** a WORM / certified compliance audit store.
- Evidence pack does **not** claim ISO/SOC certification or multi-AZ magic.
- Read-only **auditor** role is not in this slice (admin exports the pack for the auditor).

## [1.3.0] - 2026-08-03

Minor release: **cluster installer (Waves 1–2)** and **SSH Docker lab release gate** (live Apply, Patroni promote, unicast keepalived VIP — 0 SKIP), plus cluster Grafana metrics and root install entrypoints.

### Added

- **Cluster installer (Waves 1–2)** — inventory/plan/render/apply scripts (`scripts/cluster/`), templates under `deploy/cluster/templates/`, SECURITY notes, and installer tests (`scripts/tests/cluster-installer-w1|w2`).
- **SSH Docker lab release gate** — offline-capable node image + `up-ssh.sh` / `run-apply-ssh.sh` / `run-drills-ssh.sh` for live Apply, Patroni promote ≤60s, and unicast keepalived VIP move (**0 SKIP**). See `deploy/cluster/lab/README.md`.
- **Cluster observability** — Prometheus gauges `datasafe_cluster_node_*` / overall / HA; Grafana dashboard `datasafe-cluster.json`; optional Prometheus `file_sd` targets.
- **Root install entrypoints** — `install.ps1` / `install.sh` / `install.cmd` with optional `identity` profile; getting-started installer docs (EN/RU).
- **Public login options** — `GET` login-options for LDAP/OIDC console buttons without leaking secrets.
- **Compose layout** — overlays and env examples under `deploy/compose/` (paths updated in scripts/docs).

### Changed

- HAProxy cluster templates bind VIP addresses only; Patroni listens on fabric `NODE_IP` so colocated LB + Postgres do not clash.
- Lab Apply deploys storage-server after etcd/Patroni quorum; secrets file uses Patroni **superuser** password.

### Honesty

- SSH lab unicast VRRP proves the Apply/drill path on Docker Desktop; it is **not** bare-metal L2 multicast parity.
- Offline compose lab may still SKIP VIP/Patroni by design — use the SSH lab for the no-skip gate.
- Grafana cluster panels reflect `Cluster.Nodes` configured in Admin (empty → single local fallback).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.3.0`, `ghcr.io/direktorbani/datasafe-console:v1.3.0`.

## [1.2.0] - 2026-07-21

Minor release: **immutable backup golden path** (Object Lock + versioning) and audit hygiene (AUD-12 / AUD-16). Merges the former patch hygiene slice into one operator-facing minor.

### Added

- **Immutable backup use-case** — [EN](docs/use-cases/en/immutable-backup.md) / [RU](docs/use-cases/ru/immutable-backup.md); linked from backup storage and MinIO migration guides.
- **Admin/console `retention_mode`** — `GOVERNANCE` | `COMPLIANCE` on bucket Object Lock settings (GET/PUT + console selector).
- **Versioning Suspend in console** — Admin API `versioning_suspended` + UI checkbox (AC-VER-1a).
- **Feature-audit rows** — `retention_mode` round-trip, versioning suspended flag, folder-delete `object_count` on 409.
- **ADR-0003 Accepted** — [immutable backup golden path](docs/architecture/adr/0003-immutable-backup-path.md).

### Changed

- **AUD-12** — roadmap marked done (gateway public-read console indicators already shipped).
- **Roadmap** — bucket versioning marked **done** (Enabled/Suspended in console).

### Fixed

- **AUD-16** — folder delete confirm dialog and toast surface API `object_count` from HTTP 409 (`ApiError` body).

### Deferred (not in this tag)

- Scripted HA promote hardening (A5), alpine binary smoke (AUD-21), log-sink error surfacing (AUD-22) — candidate for a follow-up patch.

> Honesty: Not WORM without Object Lock enabled. Not 100% AWS Object Lock parity. Scripted HA remain lab — not automatic multi-AZ.

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.2.0`, `ghcr.io/direktorbani/datasafe-console:v1.2.0`.

## [1.1.1] - 2026-07-14

Patch release: MinIO migration kit, monitoring doc/screenshot refresh, public specs redirect, `storage-cli migrate checklist`, and AUD-19 versioning rows in feature-audit.

### Added

- **MinIO migration kit** — ops guide ([EN](docs/operations-guide/en/migrate-from-minio.md), [RU](docs/operations-guide/ru/migrate-from-minio.md)), rclone example config, cutover checklist (`internal/migrate`), smoke script [`scripts/migrate/minio-cutover-smoke.ps1`](scripts/migrate/minio-cutover-smoke.ps1), suite [`test-minio-migration-kit.ps1`](scripts/migrate/test-minio-migration-kit.ps1).
- **`storage-cli migrate checklist`** — print MinIO cutover checklist to stdout (`storage-cli migrate checklist [minio]`).
- **Architecture ADRs** — [docs/architecture/adr/](docs/architecture/adr/) (migration kit Accepted; EventSink, immutable backup, inventory, scripted promote Proposed).
- **Extension stubs** — `internal/events.EventSink`, `internal/inventory` types, `internal/ha/promote` (no runtime behavior yet).
- **AUD-19** — feature-audit: versioning enable → put two versions → list versions → get by `versionId`.

### Changed

- **Monitoring docs / screenshots** — Grafana Overview caption and HTTP/S3 + Host panel descriptions (ops, admin, user guide EN/RU); refreshed `monitoring.png` / `grafana.png`.
- **Public `docs/specs/`** — internal TZ files removed from git; [README redirect](docs/specs/README.md) points to `D:\datasafe_tz\`.

> Not a claim of 100% MinIO API parity. IAM/policies remapped manually. Object sync uses rclone/aws-cli.

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.1.1`, `ghcr.io/direktorbani/datasafe-console:v1.1.1`.

## [1.1.0] - 2026-07-05

Trust-debt and OSS-growth release: security hardening C01–C21, Teams/MFA console improvements, feature-audit coverage, **HA v2 CE lab foundation**, and **trusted cluster** multi-site pairing (mTLS, replication rules, federation `cluster_id`).

> **Lab disclaimer:** HA v2 and trusted clusters are **CE lab foundations** (compose scripts, separate ports, pairing lab). They are **not** production multi-AZ HA, not an automatic failover orchestrator, not Patroni-certified clustering, and not a turnkey multi-region product.

### Security

- **`STORAGE_OUTBOUND_HTTP_ALLOW` removed** — outbound HTTP/private targets allowed only when `STORAGE_DEV=true` (non-production). Production integrations must use public HTTPS endpoints.
- **`STORAGE_METRICS_TOKEN`** — when set, `GET /metrics` requires `Authorization: Bearer <token>`; empty token keeps legacy open mode with startup warning in production.
- **Share link tokens** — stored as SHA-256 hash only (`token_hash`); plaintext returned once on create. Postgres migration `013_share_token_hash` backfills existing links; Bolt uses hash index with legacy plaintext fallback for pre-upgrade data.
- **Pen-test preparation** — operator checklist ([EN](docs/operations-guide/en/pen-test-preparation.md), [RU](docs/operations-guide/ru/pen-test-preparation.md)) for external assessments.
- **Automated mTLS cluster pairing** — join tokens (`dsjoin_*`, 15 min, single-use) stored as **hash only**; trust via mutual TLS and CA exchange (no manual fingerprint gate).
- **Cluster PKI** — per-deployment CA and client certs on disk (`STORAGE_CLUSTER_CERT_DIR`); private keys **never** in Postgres/Bolt.
- **Cert lifecycle** — 90-day client cert TTL; leader-only rotator renews at ~75 days; revoke updates CRL and stops workers.
- **Cluster metadata at rest** — field encryption paths for cluster/site-replication secrets (`enc:v1:`).

### Added

- **Teams (admin API + console)** — `GET/POST/PUT/DELETE /api/v1/teams`, member management; Admin → Teams UI ([EN](web/console/src/locales/en/teams.json)). OpenAPI paths in `docs/api/openapi-full.yaml`.
- **MFA setup wizard** — console profile flow for TOTP enrollment and verification (`e2e/security-mfa.spec.ts` smoke).
- **Feature audit** — extended to **111** checks; Grafana panel smoke; **AUD-15** tenant matrix; **AUD-18** trash restore; trusted-cluster pairing and federation `cluster_id` slices.
- **Playwright CI regression** — 7 specs on PR (`smoke`, `buckets`, `settings`, `files`, `share`, `security-mfa`, `teams`); OIDC Keycloak browser flow moved to nightly [e2e-oidc.yml](.github/workflows/e2e-oidc.yml) (optional `E2E_OIDC_KEYCLOAK=1`, see [docs/testing/oidc-e2e.md](docs/testing/oidc-e2e.md)).
- **API guide examples** — Go S3 SDK (`docs/api-guide/en/examples/go/`) and Python Admin JWT list-buckets script; CI compile check.
- **Reference-arch backup smoke** — `scripts/reference-arch/backup-restore.ps1`; linked from [backup-storage use-case](docs/use-cases/en/backup-storage.md) (EN/RU).
- **Getting started stubs** — German (`docs/getting-started/de/`) and French (`docs/getting-started/fr/`).
- **GHCR on `main`** — `.github/workflows/publish-main.yml` pushes `:main` and `:sha-*` image tags.
- **Contributing guide** — [CONTRIBUTING.md](CONTRIBUTING.md) with local stack, Playwright list, OIDC policy, good first issues.
- **HA v2 (CE)** — erasure object backend (`STORAGE_OBJECT_BACKEND=erasure`), Postgres leader lock (`STORAGE_HA_ENABLED`), site replication Admin API + console; lab scripts under `scripts/ha/`. (Internal HA TZ moved off public `docs/specs/` — see [docs/specs/README.md](docs/specs/README.md).)
- **Trusted clusters** — `GET/POST /api/v1/clusters/…` (pairing, revoke, rotate, replication-rules); Console **Clusters** page; Playwright [`trusted-clusters.spec.ts`](web/console/e2e/trusted-clusters.spec.ts).
- **Trusted-cluster replication** — mTLS S3 client to paired peers; `STORAGE_TRUSTED_CLUSTER_REPL_ENABLED` (default `true`); migrations `017`–`019`.
- **Federation `cluster_id`** — each federation peer scoped to local or trusted remote cluster.
- **Parallel multipart uploads** — console concurrency 4 for large files.
- **Load balancer templates** — [Caddy multi-cluster LB](deploy/caddy/multi-cluster-lb.md); Helm [`caddy-lb.yaml`](deploy/helm/datasafe/templates/caddy-lb.yaml).
- **Documentation (EN/RU)** — [trusted clusters ops](docs/operations-guide/en/trusted-clusters.md), [admin console guide](docs/administrator-guide/en/trusted-clusters.md), updated user guide §8.

### Changed

- **Helm** — `storageServer.config.metricsToken` maps to `STORAGE_METRICS_TOKEN`.
- **Prometheus example** — bearer scrape config in `deploy/docker/prometheus.yml`.
- **Security self-assessment** — metrics token and outbound policy notes ([EN](docs/operations-guide/en/security-self-assessment.md), [RU](docs/operations-guide/ru/security-self-assessment.md)).
- **Site replication worker** — rules with `trusted_cluster_id` use mTLS transport.

### Migration

See [upgrade guide § v1.1.0](docs/operations-guide/en/upgrade.md#upgrading-to-v110) (EN/RU). Postgres migrations `013`–`019` apply on start. Field encryption v2 is **not** in this release.

**Trusted clusters:** set `STORAGE_CLUSTER_ID` and `STORAGE_CLUSTER_ENDPOINT` (reachable from remote site — on Docker Desktop use `host.docker.internal`, not `127.0.0.1`). Backup `{STORAGE_DATA_DIR}/cluster-certs/`. See [trusted clusters upgrade](docs/operations-guide/en/upgrade.md#trusted-clusters-post-v110).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.1.0`, `ghcr.io/direktorbani/datasafe-console:v1.1.0`.

## [1.0.3] - 2026-06-30

Trust-and-quality release: optional metadata field encryption (CE), Vault env-injection ops pattern, CI/Postgres regression hardening, security console panel, and `STORAGE_OUTBOUND_HTTP_ALLOW` sunset timeline.

### Added

- **Field encryption (metadata at rest)** — opt-in X25519 envelope for access keys, gateway credentials, and system-config secrets (`enc:v1:`). **Community Edition**, no license gate. Ops guide ([EN](docs/operations-guide/en/field-encryption.md), [RU](docs/operations-guide/ru/field-encryption.md)), Postgres migration `012_field_encryption`, [scripts/crypto/](scripts/crypto/README.md). Vault Transit / HSM for KEK — Enterprise phase 2+.
- **HashiCorp Vault (env injection)** — optional Agent / Injector pattern; maps KV v2 to existing `STORAGE_*` env (no in-app Vault SDK). Guide ([EN](docs/operations-guide/en/secrets-vault.md), [RU](docs/operations-guide/ru/secrets-vault.md)), Compose overlays (`deploy/compose/docker-compose.vault.yml`, `deploy/compose/docker-compose.vault-product.yml`), [deploy/vault/](deploy/vault/README.md), Helm [values-vault-agent.yaml](deploy/helm/datasafe/examples/values-vault-agent.yaml).
- **Console** — Admin → Settings → **Security** posture panel (`GET /api/v1/settings/security-status`, including `field_encryption` block); gateway health shows `public_read_rules` count.

### Changed

- **CI** — `e2e-smoke` mirrors feature-audit compose (`--profile postgres`, health wait); gates `e2e/smoke.spec.ts` only.
- **CI** — Postgres 16 service for `go test` with `TEST_POSTGRES_DSN`; nullable `team_id` FK integration test.
- **SSRF** — regression matrix for `STORAGE_OUTBOUND_HTTP_ALLOW` in prod vs dev.
- **Release workflow** — GitHub Release body from CHANGELOG `[version]` section (`body_path`).

### Deprecated

- **`STORAGE_OUTBOUND_HTTP_ALLOW`** — scheduled removal in **v1.1.0**; migration timeline in upgrade guide.

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.3`, `ghcr.io/direktorbani/datasafe-console:v1.0.3`.

## [1.0.2] - 2026-06-28

Security patch: SSRF hardening, production secrets guidance, object key validation, OIDC exchange flow, rate limiting, and supply-chain updates.

### Security

- **SSRF** - Outbound URL policy (`internal/security/urlpolicy`) for log sinks, hook tests, webhooks, and bucket notifications (DS-SEC-003, DS-SEC-004, DS-SEC-027).
- **OIDC** - Callback redirects with one-time `exchange_code` instead of JWT in query string; `POST /api/v1/auth/oidc/exchange` (DS-SEC-005).
- **OIDC ROPC** - `STORAGE_OIDC_ROPC_ENABLED` gates `POST /api/v1/auth/oidc/password-login` (DS-SEC-013).
- **Object keys** - `ValidateObjectKey` rejects path traversal and control characters on S3 PUT/GET/DELETE (DS-SEC-024).
- **CORS** - Configurable allowlist via `STORAGE_CORS_ALLOWED_ORIGINS` (DS-SEC-001).
- **Rate limiting** - Login endpoints limited per IP (`STORAGE_RATE_LIMIT_LOGIN`, `STORAGE_RATE_LIMIT_WINDOW`) (DS-SEC-002).
- **MFA** - Optional dedicated `STORAGE_MFA_ENCRYPTION_KEY` with JWT secret fallback (DS-SEC-026).
- **LDAP** - `STORAGE_LDAP_REQUIRE_TLS=true` rejects `ldap://` URLs in settings (DS-SEC-025).
- **Secrets** - Helm `values-production.yaml` hardening template; `GET /api/v1/settings/security-status` lists weak env vars (DS-SEC-006).
- **Go** - Toolchain bumped to Go 1.26.4; `govulncheck` in CI (DS-SEC-007).

### Changed

- Console login handles `?exchange_code=` from OIDC callback (`?token=` deprecated).
- Operator docs (EN/RU): outbound URL policy, LDAP TLS, security hardening checklist, upgrade guide v1.0.2.

### Migration

Upgrading from v1.0.1 requires a **paired** storage-server and console update. OIDC IdP redirect URIs stay the same, but the browser no longer receives a JWT in the URL; the console redeems a one-time code via `POST /api/v1/auth/oidc/exchange`. Review outbound integration URLs (Loki, Elasticsearch, webhooks): production now requires public HTTPS unless `STORAGE_OUTBOUND_HTTP_ALLOW=true` or `STORAGE_DEV=true` for local dev. Login automation may need a higher `STORAGE_RATE_LIMIT_LOGIN` or retry logic (default 10/min per IP). Rotate `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY`, and `STORAGE_ADMIN_PASSWORD` before production; use `GET /api/v1/settings/security-status` or set `STORAGE_STRICT_SECRETS=true` to fail fast on defaults. See [upgrade guide](docs/operations-guide/en/upgrade.md#upgrading-to-v102) (EN/RU).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.2`, `ghcr.io/direktorbani/datasafe-console:v1.0.2`.

## [1.0.1] - 2026-06-28

Patch release: supply-chain hygiene, documentation sync, and minor UX fixes. No new user-facing capabilities.

### Added

- SBOM (Syft CycloneDX) for **both** release images (`storage-server` and `console`).
- GitHub Release job attaches SBOM artifacts and generates release notes on version tags.
- Operator guide: `cosign verify` steps for GHCR images (EN/RU; SECURITY.md, getting-started, operations guide).

### Changed

- Docker Compose / `.env.example` / Helm examples default to `v1.0.1` GHCR tags.
- `SECURITY.md` - real security contact (`trachyk.i@gmail.com`) and GitHub Security Advisories.
- Project status and roadmap: AUD-09 (OIDC issuer unreachable) marked **done**; AUD-08 bucket list error surfacing.
- Compose project name normalization (`datasafe`) and expanded `.gitignore` (from post-v1.0.0 maintenance).
- Swagger guide: placeholder screenshots replaced with text-only steps.

### Fixed

- **AUD-08** - Buckets page shows API errors instead of a silent empty list; bucket create returns 409 only for name conflicts (500 for server/metadata errors).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.1`, `ghcr.io/direktorbani/datasafe-console:v1.0.1`.

## [1.0.0] - 2026-06-24

First public **DataSafeS3 Community Edition** release.

### Highlights

- **S3-compatible API** - buckets, objects, multipart, versioning, presigned URLs, STS session tokens, Object Lock (WORM), storage classes, and gateway replication.
- **Web console** - object browser, admin settings, tenants and groups, MFA/WebAuthn, OIDC/LDAP SSO, setup wizard, EN/RU i18n.
- **Metadata** - Bolt (dev) and PostgreSQL (production); tenant-scoped buckets and RBAC.
- **HA & operations** - Docker Compose and Helm charts, Postgres failover automation, read-replica routing, federation sync, Prometheus/Grafana dashboards.
- **Collaboration** - shared links, file collaboration (Phases 1-3), audit and webhooks.
- **Security & supply chain** - cosign image signing, govulncheck in CI, OpenAPI 3.1 spec and drift tests.
- **Documentation** - bilingual user/admin guides, API guides, and deployment cookbooks.

Container images: `ghcr.io/direktorbani/datasafe-storage-server:v1.0.0`, `ghcr.io/direktorbani/datasafe-console:v1.0.0`.

[1.5.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.5.0
[1.4.1]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.4.1
[1.4.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.4.0
[1.3.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.3.0
[1.2.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.2.0
[1.1.1]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.1.1
[1.1.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.1.0
[1.0.3]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.3
[1.0.2]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.2
[1.0.1]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.1
[1.0.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.0
