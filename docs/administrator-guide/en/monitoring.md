English | **[Русский](../ru/monitoring.md)**

# Monitoring

![Grafana — DataSafeS3 Overview dashboard](../../images/screenshots/monitoring.png)

The **DataSafeS3 Overview** dashboard has two rows:

| Row | Panels |
|-----|--------|
| **HTTP / S3** | Request rate, bucket and object counts, total storage, p95 latency, HTTP errors, top buckets by size |
| **Host** | Disk use %, CPU load (1m), memory use %, network I/O, PostgreSQL replication lag, disk capacity |

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
