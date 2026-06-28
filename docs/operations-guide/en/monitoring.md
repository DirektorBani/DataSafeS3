English | **[Русский](../ru/monitoring.md)**

# Monitoring operations

![Monitoring](../../images/screenshots/monitoring.png)

## Prometheus

- Scrape target: `storage-server:9000/metrics`
- Config: `deploy/docker/prometheus.yml`

## Grafana

- URL: http://localhost:3000 (default `admin`/`admin`)
- Dashboard: **DataSafeS3 Overview** (`deploy/docker/grafana/dashboards/datasafe-overview.json`)

## Alerts (recommended)

| Alert | Metric |
|-------|--------|
| Disk > 85% | node filesystem |
| Replication queue backlog | `datasafe_replication_queue_depth` |
| 5xx rate | HTTP metrics |
| Auth failures spike | login counter |

## External logging

Forward JSON logs to Loki/Elasticsearch for correlation with audit events.

### Outbound URL policy (v1.0.2+)

Admin-configured sink, webhook, and hook-test URLs are validated against SSRF rules (`internal/security/urlpolicy`):

- **Production** (`STORAGE_DEV=false`): only public `https://` targets (private IPs, `localhost`, and metadata IPs blocked).
- **Local dev**: set `STORAGE_DEV=true` or `STORAGE_OUTBOUND_HTTP_ALLOW=true` to allow `http://127.0.0.1` / `host.docker.internal` for Loki.
- Invalid URLs return `400` with `outbound url not allowed: …` when saving settings or testing hooks.

Full guide: [../../en/user-guide/07-monitoring-and-databases.md](../../en/user-guide/07-monitoring-and-databases.md)
