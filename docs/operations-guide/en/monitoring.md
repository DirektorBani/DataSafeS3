English | **[Русский](../ru/monitoring.md)**

# Monitoring operations

![Grafana — DataSafeS3 Overview dashboard](../../images/screenshots/monitoring.png)

Overview panels: **HTTP / S3** (RPS, counts, storage, latency, errors, top buckets), **Cluster (summary)** (overall status, healthy/offline counts, HA leader), and **Host** (disk, CPU, memory, network, Postgres replication lag).

Full per-node table and erasure health: Grafana dashboard **DataSafeS3 Cluster Status** (`deploy/docker/grafana/dashboards/datasafe-cluster.json`, uid `datasafe-cluster`).

## Prometheus

- Scrape target: `storage-server:9000/metrics`
- Config: `deploy/docker/prometheus.yml`
- **Cluster nodes (optional):** edit `deploy/docker/prometheus/targets/cluster-nodes.json` (see `cluster-nodes.example.json`) with each node `host:9000`. Compose mounts that directory into Prometheus for `file_sd`.
- **v1.1.0+:** when `STORAGE_METRICS_TOKEN` is set, configure Prometheus `authorization` bearer credentials (see `deploy/docker/prometheus.yml` comments and [upgrade guide § v1.1.0](upgrade.md#upgrading-to-v110)).
- **Console note:** the web dashboard reads `/metrics` without a bearer token; when metrics are protected, use Grafana/Prometheus for gauges (console degrades gracefully to zeros).

## Grafana

- URL: http://localhost:3000 (default `admin`/`admin`)
- Dashboards:
  - **DataSafeS3 Overview** — `datasafe-overview`
  - **DataSafeS3 Cluster Status** — `datasafe-cluster` (node UP/DOWN table, overall status, HA role, erasure degraded sets, heal rate, PG lag)
  - **DataSafeS3 Buckets** — `datasafe-buckets`
- **Feature audit (v1.1.0+):** `scripts/feature-audit-test.ps1` runs the same panel query as the overview dashboard (`sum(rate(datasafe_http_requests_total[1m]))`) against Prometheus when Grafana/Prometheus are up in the compose stack.

## Cluster metrics (Wave 2+)

Published by the storage process that runs the cluster monitor (probes configured nodes’ `/healthz`):

| Metric | Meaning |
|--------|---------|
| `datasafe_cluster_overall_status` | 2=healthy, 1=degraded, 0=offline |
| `datasafe_cluster_nodes_total` | Configured node count |
| `datasafe_cluster_nodes_healthy` | Nodes with healthy `/healthz` |
| `datasafe_cluster_nodes_offline` | Unreachable nodes |
| `datasafe_cluster_node_up{node_id,address,role}` | 1 if that node is up |
| `datasafe_cluster_node_status{…,status}` | Per-node status code |
| `datasafe_ha_enabled` / `datasafe_ha_is_leader` | Storage HA election on this process |

Configure cluster members in Admin API / console **Cluster** settings so the monitor has addresses to probe. Scraping each node’s `/metrics` (file_sd) is optional but useful when comparing host disk/CPU across members.

## Alerts (recommended)

| Alert | Metric |
|-------|--------|
| Disk > 85% | node filesystem / `datasafe_host_disk_used_percent` |
| Gateway replication queue backlog | `datasafe_replication_queue_depth` |
| Site replication lag | `datasafe_site_replication_lag_seconds` |
| Site replication queue | `datasafe_site_replication_queue_depth` |
| Erasure degraded sets | `datasafe_erasure_degraded_shard_sets` |
| Cluster node offline | `datasafe_cluster_nodes_offline > 0` or `datasafe_cluster_node_up == 0` |
| Cluster degraded | `datasafe_cluster_overall_status < 2` |
| HA not leader (unexpected) | `datasafe_ha_is_leader == 0` with writes expected |
| 5xx rate | HTTP metrics |
| Auth failures spike | login counter |

## HA v2 metrics (v1.1.0+)

| Metric | Meaning |
|--------|---------|
| `datasafe_erasure_degraded_shard_sets` | Shard sets missing shards (alert > 0) |
| `datasafe_erasure_heal_bytes_total` | Bytes rebuilt by heal worker |
| `datasafe_site_replication_lag_seconds` | Time since last completed site-replication task |
| `datasafe_site_replication_queue_depth` | Pending site-replication tasks |
| `datasafe_postgres_replication_lag_seconds` | Exposed via `/healthz` / metrics when Postgres backend is used |

Enable site replication worker: `STORAGE_SITE_REPLICATION_ENABLED=true` on the **source** site only.

## External logging

Forward JSON logs to Loki/Elasticsearch for correlation with audit events.

### Outbound URL policy (v1.0.2+)

Admin-configured sink, webhook, and hook-test URLs are validated against SSRF rules (`internal/security/urlpolicy`):

- **Production** (`STORAGE_DEV=false`): only public `https://` targets (private IPs, `localhost`, and metadata IPs blocked).
- **Local dev / CI**: set `STORAGE_DEV=true` (e.g. `deploy/compose/docker-compose.audit.yml`) to allow `http://127.0.0.1` / `host.docker.internal` for Loki. **`STORAGE_OUTBOUND_HTTP_ALLOW` was removed in v1.1.0.**
- Invalid URLs return `400` with `outbound url not allowed: …` when saving settings or testing hooks.

Full guide: [../../en/user-guide/07-monitoring-and-databases.md](../../en/user-guide/07-monitoring-and-databases.md)
