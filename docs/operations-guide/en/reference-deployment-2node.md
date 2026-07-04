# Reference deployment — HA v2 (Community Edition)

English | **[Русский](../ru/reference-deployment-2node.md)**

Community Edition HA v2 combines **erasure-coded object durability**, **orchestrated PostgreSQL metadata failover**, and optional **site replication** to another DataSafeS3 deployment. There are **no license gates** for these features.

## Topology (recommended)

```text
[Client] → Caddy → storage-server (leader, writes)
                    │
                    ├─ STORAGE_OBJECT_BACKEND=erasure (4+2 or dev 2+1)
                    │    shards on separate volumes / drives
                    │
                    ├─ Postgres primary ──streaming──► Postgres standby
                    │    ha_leader_lock (single writer)
                    │
                    └─ async trusted cluster repl ──► Site B (paired mTLS)
```

For **trust + replication** between two DataSafeS3 installs (**v1.1.0**), prefer [trusted clusters](./trusted-clusters.md) over classic site replication with access keys.

## What changed from legacy 2-node standby

| Legacy (deprecated) | HA v2 (recommended) |
|---------------------|---------------------|
| Shared FS + `storage-server-standby` read-only | Erasure shards on distinct volumes |
| Metadata-only Postgres HA | Metadata HA + leader lock + `/healthz` HA fields |
| Gateway to external S3 as primary DR | Site replication for DataSafeS3↔DataSafeS3; Gateway for external S3 |
| Manual shell-only failover | `scripts/ha/failover-metadata.ps1` + documented RPO/RTO |

Legacy read-only standby (`STORAGE_READ_ONLY=true`) remains for DR drills only — see [scaling.md](./scaling.md).

## Compose profiles (Windows lab)

| Profile | Purpose | Script |
|---------|---------|--------|
| HA metadata + 3 storage (lab) | Postgres replication + object copy sidecar | `scripts/ha/start-ha-stack.ps1` |
| Erasure backend | 6 shard volumes, single writer | `docker-compose.ha-erasure.yml` + `scripts/ha/test-erasure-backend.ps1` |
| Site replication two-stack | Site A → Site B async (AK/SK) | `scripts/ha/start-site-replication-lab.ps1` + `scripts/ha/test-site-replication.ps1` |
| **Trusted clusters two-stack** | Site A ↔ Site B mTLS pairing + repl | `scripts/ha/start-ha-stack.ps1` + `start-ha-cluster-b.ps1` + [trusted-clusters.md](./trusted-clusters.md) |

Example erasure lab:

```powershell
docker compose -p datasafe-erasure --profile postgres `
  -f docker-compose.yml -f docker-compose.local-data.yml -f docker-compose.local-binary.yml `
  -f docker-compose.ha-erasure.yml --env-file .env.ha up -d
scripts\ha\test-erasure-backend.ps1
```

## Metadata failover (manual, orchestrated)

1. Set `STORAGE_READ_ONLY=true` or stop the current leader `storage-server`.
2. Run `scripts/ha/failover-metadata.ps1` (promote Postgres standby, release leader lock).
3. Update `STORAGE_POSTGRES_DSN` on the new primary node; restart `storage-server`.
4. Verify `/healthz`: `is_leader=true`, `postgres_ok=true`, `replication_lag_s` near zero.

Automatic Patroni failover is documented as an optional compose profile — not required for CE.

## Kubernetes (Helm)

```bash
helm upgrade --install datasafe ./deploy/helm/datasafe \
  -f deploy/helm/datasafe/values-ha.yaml
```

Prefer erasure paths + Postgres HA over legacy standby Deployment. See [scaling.md](./scaling.md).

## Backup

- **Metadata:** `pg_dump` from primary or standby (prefer standby for snapshots).
- **Objects (erasure):** all shard paths under `STORAGE_ERASURE_DATA_PATHS`; snapshot each volume.
- **Objects (fs):** snapshot `STORAGE_DATA_DIR/objects/`.
- **Off-site:** Gateway replication (external S3) or site replication (peer DataSafeS3).

## Verification

```powershell
curl -s http://localhost:8082/healthz
scripts\ha\test-ha-cluster.ps1
scripts\ha\test-erasure-backend.ps1
scripts\ha\test-site-replication.ps1
scripts\dr-drill.ps1
```
