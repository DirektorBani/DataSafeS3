English | **[Русский](../ru/monitoring.md)**

# Monitoring operations

![Monitoring](../../images/screenshots/monitoring.png)

## Prometheus

- Scrape target: `storage-server:9000/metrics`
- Config: `deploy/docker/prometheus.yml`
- **v1.1.0+:** when `STORAGE_METRICS_TOKEN` is set, configure Prometheus `authorization` bearer credentials (see `deploy/docker/prometheus.yml` comments and [upgrade guide § v1.1.0](upgrade.md#upgrading-to-v110)).
- **Console note:** the web dashboard reads `/metrics` without a bearer token; when metrics are protected, use Grafana/Prometheus for gauges (console degrades gracefully to zeros).

## Grafana

- URL: http://localhost:3000 (default `admin`/`admin`)
- Dashboard: **DataSafeS3 Overview** (`deploy/docker/grafana/dashboards/datasafe-overview.json`)
- **Feature audit (v1.1.0+):** `scripts/feature-audit-test.ps1` runs the same panel query as the overview dashboard (`sum(rate(datasafe_http_requests_total[1m]))`) against Prometheus when Grafana/Prometheus are up in the compose stack.

## Alerts (recommended)

| Alert | Metric |
|-------|--------|
| Disk > 85% | node filesystem |
| Gateway replication queue backlog | `datasafe_replication_queue_depth` |
| Site replication lag | `datasafe_site_replication_lag_seconds` |
| Site replication queue | `datasafe_site_replication_queue_depth` |
| Erasure degraded sets | `datasafe_erasure_degraded_shard_sets` |
| HA not leader (unexpected) | `/healthz` `is_leader=false` with writes expected |
| 5xx rate | HTTP metrics |
| Auth failures spike | login counter |

## HA v2 metrics (v1.1.0+)

| Metric | Meaning |
|--------|---------|
| `datasafe_erasure_degraded_shard_sets` | Shard sets missing shards (alert > 0) |
| `datasafe_erasure_heal_bytes_total` | Bytes rebuilt by heal worker |
| `datasafe_site_replication_lag_seconds` | Time since last completed site-replication task |
| `datasafe_site_replication_queue_depth` | Pending site-replication tasks |
| `postgres_replication_lag_seconds` | Exposed via `/healthz` when Postgres backend is used |

Enable site replication worker: `STORAGE_SITE_REPLICATION_ENABLED=true` on the **source** site only.

## External logging

Forward JSON logs to Loki/Elasticsearch for correlation with audit events.

### Outbound URL policy (v1.0.2+)

Admin-configured sink, webhook, and hook-test URLs are validated against SSRF rules (`internal/security/urlpolicy`):

- **Production** (`STORAGE_DEV=false`): only public `https://` targets (private IPs, `localhost`, and metadata IPs blocked).
- **Local dev / CI**: set `STORAGE_DEV=true` (e.g. `docker-compose.audit.yml`) to allow `http://127.0.0.1` / `host.docker.internal` for Loki. **`STORAGE_OUTBOUND_HTTP_ALLOW` was removed in v1.1.0.**
- Invalid URLs return `400` with `outbound url not allowed: …` when saving settings or testing hooks.

Full guide: [../../en/user-guide/07-monitoring-and-databases.md](../../en/user-guide/07-monitoring-and-databases.md)
