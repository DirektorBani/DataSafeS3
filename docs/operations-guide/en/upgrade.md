English | **[Русский](../ru/upgrade.md)**

# Upgrade

## Docker Compose

```bash
git pull
docker compose --profile postgres build storage-server
docker compose --profile postgres up -d
```

With local binary overlay (Windows dev):

```cmd
scripts\dev-docker-local-binary.cmd
```

## Migrations

PostgreSQL schema migrations run automatically on `storage-server` start (`internal/metadata/postgres/migrations/`).

## Rollback

1. Stop stack
2. Restore previous binary/image and data backup
3. Start stack

## Verify release images (cosign)

Before upgrading to a tagged release, verify GHCR signatures (see [SECURITY.md](../../../SECURITY.md)):

```bash
export COSIGN_EXPERIMENTAL=1
TAG=v1.0.3
cosign verify "ghcr.io/direktorbani/datasafe-storage-server:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
cosign verify "ghcr.io/direktorbani/datasafe-console:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

SBOM files are attached to each [GitHub Release](https://github.com/DirektorBani/DataSafeS3/releases).

## Upgrading to v1.0.2

Release **v1.0.2** is a security patch. There are no new product features, but several defaults and auth flows change. Plan a short maintenance window and upgrade **storage-server and console together** — a v1.0.2 server with an older console (or the reverse) breaks OIDC login.

### Why this patch matters

Earlier releases put the OIDC session JWT in the browser redirect URL (`?token=…`). That token could leak via browser history, proxy logs, or referrer headers. v1.0.2 replaces it with a one-time `exchange_code` that the console redeems over POST. Separately, the server now validates every outbound HTTP URL it opens (log sinks, webhooks, hook tests, gateway checks) to block SSRF toward private networks. Login endpoints also get per-IP rate limits, and production deployments get clearer warnings when default secrets are still in place.

### OIDC callback (breaking for mixed versions)

The IdP redirect URI is unchanged (`/api/v1/auth/oidc/callback`). After login, the server redirects the browser to `/login?exchange_code=…&auth_source=oidc` instead of `?token=…`. The console calls `POST /api/v1/auth/oidc/exchange` with the code and stores the JWT from the response body. Upgrade both images before the next SSO login. Custom clients that parsed `?token=` must switch to the exchange endpoint (documented in [`openapi-full.yaml`](../../api/openapi-full.yaml)).

### Outbound URLs (SSRF policy)

In production (`STORAGE_DEV` unset or false), server-initiated HTTP must use **public HTTPS** endpoints. Plain `http://`, loopback, and RFC1918 targets are rejected unless you explicitly relax policy.

For local Loki on `http://localhost:3100`, either keep `STORAGE_DEV=true` or set `STORAGE_OUTBOUND_HTTP_ALLOW=true` (temporary escape hatch — see [planned deprecation](#planned-deprecation-storage_outbound_http_allow) below). The `deploy/compose/docker-compose.audit.yml` overlay sets relaxed outbound and higher login limits for feature-audit runs; use it only in dev/CI, not in production.

### Planned deprecation: `STORAGE_OUTBOUND_HTTP_ALLOW`

| Release | Change |
|---------|--------|
| **v1.0.2** | Strict outbound URL policy (`internal/security/urlpolicy`); escape hatch via `STORAGE_OUTBOUND_HTTP_ALLOW=true` |
| **v1.0.3** | Documented sunset timeline (this section); default remains `false` in production |
| **v1.1.0** | **`STORAGE_OUTBOUND_HTTP_ALLOW` removed** — use `STORAGE_DEV=true` only on non-production stacks, or point integrations at public **HTTPS** endpoints |

**Before v1.1.0:** audit compose, Helm values, and `.env` for `STORAGE_OUTBOUND_HTTP_ALLOW=true`. Replace with HTTPS URLs where possible; for local Loki/Elasticsearch use `STORAGE_DEV=true` on dev/CI overlays only (`deploy/compose/docker-compose.audit.yml`, not production).

**Production checklist:** unset `STORAGE_OUTBOUND_HTTP_ALLOW` (or leave at default `false`); confirm log sinks and webhooks use `https://` public endpoints; run `GET /api/v1/settings/security-status` after upgrade.

### Login rate limits

Default: **10** login attempts per IP per minute (`STORAGE_RATE_LIMIT_LOGIN`, window `STORAGE_RATE_LIMIT_WINDOW`, default `1m`). CI scripts and load tests may hit 429 — raise the limit in a test overlay (see `deploy/compose/docker-compose.audit.yml`) or add backoff in automation.

### New and changed environment variables

Review `.env.example` and rotate anything still at dev defaults:

| Variable | Default (prod) | Operator note |
|----------|----------------|---------------|
| `STORAGE_OUTBOUND_HTTP_ALLOW` | `false` | Allow non-HTTPS outbound (dev/Loki only) |
| `STORAGE_OIDC_ROPC_ENABLED` | `false` | Resource-owner password grant; enable only for test IdPs |
| `STORAGE_LDAP_REQUIRE_TLS` | `true` | Rejects `ldap://` URLs saved in LDAP settings |
| `STORAGE_MFA_ENCRYPTION_KEY` | (falls back to JWT secret) | Separate key for MFA secret encryption at rest |
| `STORAGE_CORS_ALLOWED_ORIGINS` | (empty) | Comma-separated browser origins for the console |
| `STORAGE_RATE_LIMIT_LOGIN` | `10` | Max auth attempts per IP per window |
| `STORAGE_RATE_LIMIT_WINDOW` | `1m` | Sliding window for login rate limit |
| `STORAGE_STRICT_SECRETS` | `false` | When `true`, refuse startup if default JWT/admin/S3 secrets remain |

Pre-flight check after upgrade: `GET /api/v1/settings/security-status` (admin JWT) lists any weak env vars still in use.

### Upgrade steps

```bash
git pull
export TAG=v1.0.3   # or build from source
docker compose --profile postgres pull   # if using GHCR images
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # or pull datasafe-console:v1.0.3
docker compose --profile postgres up -d
```

Verify cosign signatures with `TAG=v1.0.3` (see below), then smoke-test local login, OIDC (if used), and one outbound integration (webhook or log sink).

## Upgrading to v1.0.3

Release **v1.0.3** is a trust-and-quality patch (CI smoke, Postgres FK regression, SSRF tests, optional Vault ops pattern). **Field encryption** is new but **opt-in** — default behaviour matches v1.0.2 until you enable it.

### Field encryption (optional)

| Topic | Action |
|-------|--------|
| Default | `STORAGE_FIELD_ENCRYPTION_ENABLED=false` — no change required on upgrade |
| Postgres | Migration `012_field_encryption` runs automatically (`encryption_key_registry` table) |
| Enable | Generate KEK → set env → restart; see [field-encryption.md](field-encryption.md) |
| Verify | `GET /api/v1/settings/security-status` → `field_encryption` block; Admin → Settings → Security |
| Vault | Agent can inject `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` like other bootstrap secrets ([secrets-vault.md](secrets-vault.md)) |

**Community Edition:** env/file KEK, no license gate. Vault Transit / HSM for KEK — Enterprise phase 2.

Local key tooling: [scripts/crypto/README.md](../../../scripts/crypto/README.md).

### v1.0.3 environment variables (field encryption)

| Variable | Default | Operator note |
|----------|---------|---------------|
| `STORAGE_FIELD_ENCRYPTION_ENABLED` | `false` | `true` = encrypt selected metadata secrets at rest |
| `STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID` | (unset) | Required when enabled; must match active registry row |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` | (unset) | Base64 raw X25519 private seed |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS` | (unset) | JSON map for rotation (multiple private keys) |

### Upgrade steps (v1.0.3)

```bash
git pull
export TAG=v1.0.3
docker compose --profile postgres pull   # if using GHCR images
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # or pull datasafe-console:v1.0.3
docker compose --profile postgres up -d
```

Verify cosign with `TAG=v1.0.3` (see [Verify release images](#verify-release-images-cosign)), then `GET /api/v1/settings/security-status` and Admin → Settings → Security. Enable field encryption only after [field-encryption.md](field-encryption.md) checklist.

## Upgrading to v1.1.0

Release **v1.1.0** is a **trust-debt** minor: closes scheduled SSRF escape hatch removal and adds optional **metrics endpoint authentication**. Upgrade **storage-server and console together** (console has no breaking API changes, but paired releases simplify support).

### Outbound URLs (breaking)

| Before (v1.0.x) | After (v1.1.0) |
|-----------------|----------------|
| `STORAGE_OUTBOUND_HTTP_ALLOW=true` allowed loopback HTTP in production | **Variable removed** — ignored if still set in `.env` |
| Production Loki at `http://127.0.0.1:3100` with allow flag | Use **HTTPS** public endpoint, or `STORAGE_DEV=true` **only** on dev/CI overlays |

**Dev/CI:** `deploy/compose/docker-compose.audit.yml` sets `STORAGE_DEV=true` for feature-audit and local Loki loops — do **not** use this overlay in production.

**Checklist:**

1. Remove `STORAGE_OUTBOUND_HTTP_ALLOW` from compose, Helm, and `.env`.
2. Audit log sinks, webhooks, and hook tests — switch to `https://` where they pointed at private HTTP.
3. For local stacks, confirm `STORAGE_DEV=true` only on non-production hosts.

### Metrics authentication (breaking when token set)

| `STORAGE_METRICS_TOKEN` | Behaviour |
|-------------------------|-----------|
| Unset or empty | `/metrics` remains open (legacy); server logs a **warning** when `STORAGE_DEV=false` |
| Non-empty | `GET /metrics` requires `Authorization: Bearer <token>` |

**Prometheus scrape example:**

```yaml
scrape_configs:
  - job_name: storage-server
    static_configs:
      - targets: ["storage-server:9000"]
    authorization:
      type: Bearer
      credentials: "<STORAGE_METRICS_TOKEN>"
```

**Helm:** set `storageServer.config.metricsToken` in values (stored in the storage-server Secret).

**Console dashboard:** when metrics are token-protected, the console may show zero bucket/storage gauges (it does not send the bearer token). Use Grafana/Prometheus for operational metrics.

### Share link tokens (metadata)

| Backend | Behaviour after upgrade |
|---------|----------------------|
| **PostgreSQL** | Migration `013_share_token_hash` backfills `token_hash` for existing rows; plaintext `token` column dropped after backfill |
| **Bolt** | New links stored as hash only; server keeps a **legacy plaintext index fallback** so pre-upgrade share URLs keep working until links are recreated |

No operator action required for existing Postgres links. Bolt dev installs: old plaintext-indexed links remain valid; newly created links use hash storage only.

### New and changed environment variables

| Variable | Change |
|----------|--------|
| `STORAGE_OUTBOUND_HTTP_ALLOW` | **Removed** |
| `STORAGE_METRICS_TOKEN` | **Enforced** on `/metrics` when non-empty |

### Upgrade steps (v1.1.0)

```bash
git pull
export TAG=v1.1.0   # or build from source
docker compose --profile postgres pull
docker compose --profile postgres build storage-server
scripts/build-console.cmd
docker compose --profile postgres up -d
```

1. Apply migration checklist above **before** or immediately after restart.
2. Update Prometheus scrape config if using `STORAGE_METRICS_TOKEN`.
3. Run `GET /api/v1/settings/security-status` and feature-audit on staging.

**Not in v1.1.0:** field encryption v2, mobile GA, Vault Transit in-process KEK — see [project status](../../en/context/project-status.md).

## Trusted clusters (post-v1.1.0)

Included in **v1.1.0**. Adds mTLS pairing, trusted remote clusters, and replication rules scoped to paired sites.

### PostgreSQL migrations

| Migration | Content |
|-----------|---------|
| `017` | `trusted_clusters`, pairing sessions, cluster certificates |
| `018` | `federation_clusters.cluster_id` |
| `019` | `site_replication_rules.trusted_cluster_id` |

Migrations apply automatically on `storage-server` start. No manual SQL required.

### New environment variables

| Variable | Default | Note |
|----------|---------|------|
| `STORAGE_CLUSTER_ID` | `local` | Unique site id |
| `STORAGE_CLUSTER_ENDPOINT` | — | Must be reachable **from remote site** (see Docker note below) |
| `STORAGE_CLUSTER_CERT_DIR` | `{data}/cluster-certs` | Backup with TLS material |
| `STORAGE_TRUSTED_CLUSTER_REPL_ENABLED` | `true` | Trusted-cluster replication worker |

### Operator checklist

1. Set `STORAGE_CLUSTER_ID` and `STORAGE_CLUSTER_ENDPOINT` on **both** sites before pairing.
2. On Docker: use `host.docker.internal` (or real DNS), not `127.0.0.1`, for cross-container pairing.
3. Pair via console **Clusters** or API; verify both sides show **healthy**.
4. Add replication rules on initiator; test PUT/DELETE.
5. Backup `cluster-certs` directory.

Full guide: [trusted-clusters.md](trusted-clusters.md) (EN) · [RU](../ru/trusted-clusters.md)

## Upgrading to v1.4.1

Patch on **v1.4.0**. No database migration. Take storage-server and console from the same tag.

### Dev mode is opt-in

| Before (v1.4.0 compose default) | After (v1.4.1) |
|---------------------------------|----------------|
| `STORAGE_DEV` defaulted to `true` | Default is `false` in `docker-compose.yml` and `.env.example` |
| Installer left dev mode on | Installer writes `STORAGE_DEV=false` unless `-Dev`, `--dev`, or profile `dev` |
| Default install left `/metrics` open | Installer generates `STORAGE_METRICS_TOKEN`; Prometheus scrapes with that bearer |

If `.env` already sets `STORAGE_DEV=true`, that value stays until you change it. A stack that inherited the old compose default (variable unset) leaves dev mode on the next `up`.

Opt in for local HTTP sinks: `-Dev` / `--dev` / profile `dev`, `STORAGE_DEV=true`, or the dev and audit overlays (`deploy/compose/docker-compose.dev.yml`, `docker-compose.audit.yml`), which set `STORAGE_DEV=true` explicitly.

The **identity** lab profile sets `STORAGE_DEV=true` because the lab Keycloak issuer is plain HTTP. Do not use that profile on a production host.

The first-run wizard is unchanged: sign in as `admin` / `admin`, change the password, finish setup (external S3 is optional). The installer does not rotate `STORAGE_ADMIN_PASSWORD`. It replaces `STORAGE_JWT_SECRET` and `STORAGE_SECRET_KEY` only while they are still the published defaults, and it generates `STORAGE_METRICS_TOKEN` when that value is missing. A later run keeps non-default secrets.

Manual `docker compose` without the installer: set `STORAGE_METRICS_TOKEN` yourself. While it is empty, `/metrics` stays open (legacy) and the server logs a warning when `STORAGE_DEV=false`. When the variable is set, the Prometheus service reads the same value.

Grafana on an installer-managed stack keeps working because Prometheus sends the bearer. The console does not; use Grafana for gauges.

### Windows source builds

`.gitattributes` forces LF for `*.sh` and for the other files copied or bind-mounted into Linux containers. Rebuild the storage-server image after pulling. The image strips CR from `docker-entrypoint.sh` and starts through `/usr/local/bin/datasafe-entrypoint`, so a CRLF working copy still boots.

An existing Windows checkout does not rewrite files that this pull does not touch. Refresh shell scripts once so bind-mounted Postgres init (`deploy/docker/postgres/init/02-replication-hba.sh`) is LF before the first database init:

```bash
git checkout -- "*.sh"
```

### Upgrade steps (v1.4.1)

```bash
git pull
export TAG=v1.4.1
docker compose --profile postgres pull
docker compose --profile postgres up -d
```

Re-run `install.ps1` / `install.sh` if you want generated secrets and the metrics token wired for you. Confirm `GET /metrics` without a bearer is **401** when the token is set, and that Grafana still shows the Overview dashboard.

## Upgrading to v1.5.0

Minor release on **v1.4.1**. Take storage-server and console from the same tag. `GET /healthz` and `GET /api/v1/health` report `version` and `commit`.

Images `ghcr.io/direktorbani/datasafe-storage-server:v1.5.0` and `datasafe-console:v1.5.0` appear when `release.yml` runs on the git tag. This repository commit does not push that tag.

### Breaking and operator notes

| Topic | What changes |
|-------|----------------|
| Postgres | Migration `020` creates `event_outbox`. Migration `021` adds gateway sync-schedule columns. Both run on startup. A rollback to a v1.4.x binary leaves them in place and ignores them. The down migrations drop the table (and the events in `020`) or the schedule columns. |
| Bolt | Buckets `event_outbox` and `event_outbox_due` are created the next time the database opens. v1.4.x ignores unknown buckets. |
| Sinks | The no-op Kafka and RabbitMQ stubs are gone. Kafka delivers only when `STORAGE_KAFKA_BROKERS` is set. An empty value does not dial. Webhook and NATS URLs are unchanged. |
| Lab ports | Erasure defaults are console **8084**, S3 **9004**, Postgres **5444**. Site A defaults are console **8086**, S3 **9006**, Postgres **5446**. Delete an old `.env.erasure` or `.env.site-a` and copy the example again. A copied file keeps the previous ports. |
| Takeover | A follower takes the leader lease only after it expires. Time is about the remaining lease (about 9s for a 10s lease, about 26s for the default 30s). A process that already held the lease and then lost it does not acquire it again. |

Webhook, NATS, and Kafka are an external event feed, not the link between storage nodes. See [events](../../en/integrations/events.md) and [high availability](high-availability.md).

### What v1.4.1 hosts gain

Pulling v1.5.0 onto a v1.4.1 host is forward-compatible for data:

- Postgres applies migration `020` (`event_outbox`) on startup. A rollback to a v1.4.x binary leaves the table in place and ignores it. The down migration drops the table and the events in it.
- Bolt creates buckets `event_outbox` and `event_outbox_due` the next time the database opens. v1.4.x ignores unknown buckets.
- `STORAGE_EVENTS_RETENTION` (`24h`), `STORAGE_EVENTS_DEAD_RETENTION` (`168h`), `STORAGE_EVENTS_MAX_PENDING` (`100000`), and `STORAGE_EVENTS_MAX_ATTEMPTS` (`20`) are optional. Unset keeps those defaults. `MAX_PENDING=0` disables the cap.
- Webhook settings and `STORAGE_NATS_URL` / `STORAGE_NATS_SUBJECT` are unchanged. An empty NATS URL still does not connect. Single-node bolt with default config does not open a broker connection.
- Admin events and S3 SigV4 object events (put, copy, delete including delete markers, multipart complete) are delivered from the outbox (at-least-once) when a sink is configured. With no `STORAGE_NATS_URL`, no enabled webhook, and no matching bucket notification, those writes do not insert an outbox row and are not replayed later.
- Bucket-notification webhooks use the same dispatcher. The POST body is still `{"event","bucket","key","size","time"}`. Optional prefix/suffix filters apply at insert. `time` is the commit time of the object record. One URL is still stored (last topic, then last queue).
- Webhook retry: transport errors, timeouts, 5xx, 408, and 429 are retried (`Retry-After` is honoured, capped at 5 minutes). Any other 4xx marks that sink dead immediately; `last_error` contains the status. Each console webhook still keeps one history row per event.
- NATS uses JetStream publish-with-ack when the server has JetStream and a stream covers `STORAGE_NATS_SUBJECT.<event>`. Otherwise it is core publish, which does not confirm that a subscriber received the message. There is no extra environment variable; the choice is detected on connect and cached until reconnect.

Details: [event-outbox.md](event-outbox.md).

### Kafka (opt-in)

No metadata migration beyond `020` and `021`.

- Leave `STORAGE_KAFKA_BROKERS` empty to keep the previous behaviour: no Kafka client and no connection. The installer profile `kafka` is off unless you pass it. Default `--yes` profiles do not include it.
- A non-empty broker list registers a franz-go producer (`acks=all`, idempotent, topic `STORAGE_KAFKA_TOPIC` default `datasafe.events`). SASL `PLAIN` without TLS refuses to start. SCRAM-SHA-256 and SCRAM-SHA-512 are accepted. Username and password may be supplied with `STORAGE_KAFKA_SASL_USERNAME_FILE` and `STORAGE_KAFKA_SASL_PASSWORD_FILE`.
- Webhook and NATS payloads are unchanged. Kafka messages are a separate JSON document (`Records[]` plus `datasafe`).
- The dispatcher now holds per-object order on every sink. A stuck key blocks only that key. If an earlier row goes `dead`, later rows for that key are delivered and the dead one is a gap.
- A change in the broker list between process starts is written to Activity as `kafka_brokers_changed` (the new addresses, not SASL secrets).
- Console: **Settings → Event delivery**. `GET /api/v1/events/delivery` and `POST /api/v1/events/test` are administrator-only.

Details: [kafka.md](../../en/integrations/kafka.md).

### Upgrade steps (v1.5.0)

After the git tag `v1.5.0` is published:

```bash
git pull
export TAG=v1.5.0
# .env (or the shell) should name the published tag images:
# DATASAFE_SERVER_IMAGE=ghcr.io/direktorbani/datasafe-storage-server:v1.5.0
# DATASAFE_CONSOLE_IMAGE=ghcr.io/direktorbani/datasafe-console:v1.5.0
docker compose --profile postgres pull
docker compose --profile postgres up -d
```

Pull the `v1.5.0` server and console images. Do not `docker compose build` for this upgrade. Confirm `GET /healthz` returns `"version":"1.5.0"` and a `commit`, and `GET /` on the console port returns the SPA (`id="root"`). On a 3-node lab, delete `.env.erasure` and `.env.site-a` before recreating those stands.

**Known issue, fixed in 1.6.0.** On 1.5.x, `STORAGE_SSE_MASTER_KEY` breaks object writes. The backend compares the plaintext `Content-Length` with the longer ciphertext and returns a size mismatch, so no SSE object is stored. Do not set the variable on 1.5.x. 1.6.0 writes a format-1 envelope and reports the plaintext size. Objects created on 1.5.x without the key stay plaintext when 1.6.0 starts with a key, including zip, gzip, jpeg, and other binary files. 1.5.0 did not record whether an object was encrypted. 1.6.0 classifies each unmarked object once: it tries the server key, stores `plaintext: true` when the tag does not match, and stores format 0 when it does. Startup walks the unmarked set in the background. A read before that walk classifies that object and then serves it. Later reads use the marker and do not guess. Plaintext is streamed from the file. A format-0 object that authenticated and is later changed is HTTP 500 `cipher: message authentication failed`, and the connection is closed. A blob that has never authenticated is not treated as ciphertext. `STORAGE_SSE_LEGACY_FORMAT0` does not force that reader onto every unmarked object; the result is the same with the variable set or unset. A format-1 object written by 1.6.0 is authenticated before HTTP 200. A changed byte is the same 500. GET does not answer 200 and then leave the connection open with an empty body.

## Upgrading to v1.6.0

Finish the rolling upgrade of every storage node before you turn bucket encryption on. A 1.5.0 process cannot read a format-1 object (`DSSE`). Upgrade every node first. Upgrading the leader last is not enough: a 1.5.0 leader runs 1.5.0 code and accepts `PutBucketEncryption` itself. That process has no 409 gate.

## Mixed cluster

HTTP 409 `cluster_upgrade` (`cluster upgrade in progress`) is returned by a 1.6.0 node while any live 1.5.0 peer is still in the cluster. The check does not require `system_config` → `cluster.nodes`. A rolling upgrade does not have to register the old nodes there.

| What is visible | 409 |
|---|---|
| Heartbeat in `cluster_node_heartbeats` whose version is below 1.6.0, or a non-blank version that does not parse as 1.6.0 or newer | Yes. An unknown version is treated as older than 1.6.0. A blank version left by an address registration is not a version. |
| Unexpired HA leader lease `ha_leader_lock` (`lock_id=storage-server`, `holder_id`, `acquired_at`, `expires_at`) whose `holder_id` has no 1.6.0 heartbeat | Yes. This is the 1.5.0 leader. |
| Recent row in `ha_leader_attempts` whose `holder_id` has no 1.6.0 heartbeat | Yes. This is a live 1.5.0 follower. |
| Cluster registry node whose `GET /healthz` is `"status":"ok"` and `"version":"1.5.0"`, or a healthy registry node with an empty version | Yes, when that list is populated. An unreachable registry entry does not block. |

1.5.0 does not write `cluster_node_heartbeats`. A follower retries the same lease `INSERT` as the leader (`TryAcquireLeader`). While the lease is held the `WHERE` clause does not match, so `ha_leader_lock` stays with the leader. Migration `030` adds `ha_leader_attempts` and a `BEFORE INSERT` trigger on `ha_leader_lock`. That trigger runs for the follower statement and stores `holder_id` even when the update is skipped. A 1.6.0 peer that retries the lease also appears there, and its heartbeat is at least 1.6.0, so it does not block. A stopped node leaves the two-minute window.

While the gate is on, a PUT with no encryption header stays plaintext. SSE-S3 requested in that window is format 0, which a 1.5.0 reader with the same server key can open. SSE-KMS is HTTP 409.

On Postgres or HA, the console asks you to check **All nodes are upgraded to 1.6.0** before it sends the admin `PUT`. S3 `PutBucketEncryption` has no such checkbox. It uses the same 409 rules as the table. Do not enable encryption until every node is 1.6.0. A 1.5.0 leader runs 1.5.0 code and accepts `PutBucketEncryption` itself, so the call has to reach a 1.6.0 process.

`GET /healthz` on a 1.6.0 process reports at least `1.6.0`, including when the image stamp still says 1.5.0. A stopped 1.5.0 leader drops its lease within about 30 seconds.

If `STORAGE_SSE_MASTER_KEY` was set and a later start clears it, the process refuses to start. The error says the key is missing and to set `STORAGE_SSE_MASTER_KEY` to the original key. A different value still reports `SSE master key fingerprint mismatch`.

Vault Transit stays off until `STORAGE_KMS_PROVIDER=vault-transit` and an address are both set, in the environment or under Settings → Vault (KMS). With those empty, the process does not open a Vault connection. SSE-S3 keeps using the server key and does not call Vault. See [kms-vault.md](kms-vault.md).

## Checklist

- [ ] Backup metadata and objects
- [ ] Review changelog / migrations
- [ ] Test on staging
- [ ] Rebuild console if UI changed: `scripts\build-console.cmd`
- [ ] v1.0.2: upgrade server **and** console together for OIDC
- [ ] v1.0.2: review outbound URLs and `STORAGE_RATE_LIMIT_LOGIN` for automation
- [ ] v1.0.3 (optional): enable [field encryption](field-encryption.md) — generate KEK, set env, verify security-status
- [ ] v1.1.0: remove `STORAGE_OUTBOUND_HTTP_ALLOW`; configure `STORAGE_METRICS_TOKEN` + Prometheus bearer if used
- [ ] v1.4.1: `STORAGE_DEV` is false unless you opted in; installer or `.env` has `STORAGE_METRICS_TOKEN`; Prometheus scrape still works
- [ ] v1.5.0: `docker compose pull` the `v1.5.0` server and console images (do not build them); delete old `.env.erasure` / `.env.site-a` if those labs should use the new ports; leave `STORAGE_KAFKA_BROKERS` empty unless you want the external feed
- [ ] v1.6.0: upgrade every node before enabling encryption. 409 covers a 1.5.0 leader and a live 1.5.0 follower (lease attempt, unknown version treated as old). `cluster.nodes` is not required. A 1.5.0 leader accepts its own `PutBucketEncryption` when the call reaches that process. On Postgres or HA, check “All nodes are upgraded to 1.6.0” in the console. S3 `PutBucketEncryption` has no checkbox.
- [ ] v1.6.0: leave Vault unset unless you use it. The client starts only when `STORAGE_KMS_PROVIDER=vault-transit` and an address are both set. SSE-S3 does not call Vault. See [kms-vault.md](kms-vault.md).
