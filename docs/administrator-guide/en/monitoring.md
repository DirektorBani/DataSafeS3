English | **[Русский](../ru/monitoring.md)**

# Monitoring

![Monitoring](../../images/screenshots/monitoring.png)

## Stack

| Component | URL | Role |
|-----------|-----|------|
| Prometheus | http://localhost:9090 | Scrapes `/metrics` |
| Grafana | http://localhost:3000 | Dashboard **DataSafeS3 Overview** |

## Key metrics

- HTTP RPS and latency
- Storage bytes, bucket/object counts
- S3 read/write operations
- Replication queue depth
- Host CPU, memory, disk (Linux)

## Console

Usage page shows per-user consumption. Gateway page shows replication health.

## Full guide

[Monitoring and databases](../../en/user-guide/07-monitoring-and-databases.md) · [Operations guide](../../operations-guide/en/monitoring.md)
