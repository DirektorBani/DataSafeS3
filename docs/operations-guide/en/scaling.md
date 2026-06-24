English | **[Русский](../ru/scaling.md)**

# Scaling

DataSafeS3 Community Edition is **single-node by default**. This page describes what you can do today without overpromising HA features.

## Single-node by default

| Approach | Status | Notes |
|----------|--------|-------|
| One `storage-server` + local/Postgres metadata | **Implemented** | Default deployment model |
| Vertical scaling (CPU/RAM/disk) | **Implemented** | Primary scaling path today |
| Gateway replication to external S3 | **Implemented** | Off-site copies, not active-active HA |
| Federation (multi-cluster awareness) | **Partial** | Cross-node GetObject proxy when peers registered |
| PostgreSQL read replicas for metadata | **Implemented** | `STORAGE_POSTGRES_READ_REPLICA_DSN` routes list queries |
| Multi-AZ / erasure-coded storage tier | **Partial** | Erasure 2+1 MVP in `internal/storage/erasure/`; not petabyte parity |

Do **not** assume automatic failover or multi-AZ durability — verify against the table above and [architecture](../../en/context/architecture.md).

## Vertical scaling

- Increase CPU/RAM for `storage-server`
- Attach faster/larger disks to `STORAGE_DATA_DIR`
- Use PostgreSQL for metadata under higher concurrency

## PostgreSQL metadata HA (active-passive)

Community Edition supports **manual** PostgreSQL streaming replication for metadata. There is no automatic failover orchestrator.

### Primary + standby setup

1. Install PostgreSQL 15+ on primary and standby hosts.
2. On primary, enable WAL archiving and replication user:

```sql
CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'changeme';
```

3. Configure `postgresql.conf` on primary: `wal_level=replica`, `max_wal_senders=5`, `hot_standby=on`.
4. Add standby entry to `pg_hba.conf` for the replicator role.
5. On standby, take a base backup and create `standby.signal` + `primary_conninfo` in `postgresql.auto.conf` (standard PostgreSQL streaming replication).
6. Point DataSafeS3 primary `storage-server` at the primary DSN (`STORAGE_POSTGRES_DSN` or host vars).
7. Optionally set `STORAGE_POSTGRES_READ_REPLICA_DSN` on the primary app node to offload **list buckets** and **list objects** metadata queries to the standby.

### Health and lag

- `GET /healthz` includes `postgres_ok` and `postgres_replication_lag_seconds` when Postgres is the metadata backend.
- Alert when lag exceeds your RPO (Grafana panel in bundled dashboards).

### Manual failover (metadata)

1. Stop writes: set `STORAGE_READ_ONLY=true` on the old primary `storage-server` or stop the process.
2. Promote standby: `pg_ctl promote` or `SELECT pg_promote();` on the replica.
3. Update `STORAGE_POSTGRES_DSN` (and remove or repoint read replica DSN) on all `storage-server` instances.
4. Restart `storage-server` and verify `/healthz` and console login.
5. Rebuild replication: former primary becomes new standby if you need read scaling again.

See also [disaster-recovery](./disaster-recovery.md).

## Read-only storage-server standby

Set `STORAGE_READ_ONLY=true` on a secondary `storage-server` sharing the same metadata + object data path (or replica metadata + replicated object store). Mutating Admin/S3 APIs return **503** with `Retry-After`; GET/List/Head remain available for DR read access. Example overlay: `docker-compose.ha.yml`. **Community Edition — full HA tooling** (failover scripts, DR drill, Helm `values-ha.yaml`) — see [reference deployment](./reference-deployment-2node.md).

## Horizontal options (limited today)

| Approach | Status | Notes |
|----------|--------|-------|
| **Gateway replication** | Implemented | Offload copies to external S3 |
| **Federation (MVP)** | Implemented | Register peers; S3 GetObject + **ListObjectsV2** prefix proxy — [federation docs](../../en/user-guide/08-federation-and-cluster.md) |
| **Read replicas** | Implemented | Postgres list routing via `STORAGE_POSTGRES_READ_REPLICA_DSN` |

## Kubernetes

Helm chart supports resource limits, PDB, and production values (`values-production.yaml`). See [deploy/helm/datasafe/README.md](../../../deploy/helm/datasafe/README.md).

For hyper-scale multi-petabyte needs, use Gateway replication or erasure MVP today; see [reference deployment](./reference-deployment-2node.md).

Benchmarks: [performance-benchmarks](../../testing/performance-benchmarks.md).
