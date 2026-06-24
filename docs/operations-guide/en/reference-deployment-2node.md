# Reference deployment — 2-node HA + backup (Community Edition)

English | **[Русский](../ru/reference-deployment-2node.md)**

This guide describes a **supported Community Edition** pattern: active-passive PostgreSQL metadata, optional read-only `storage-server` standby, and external backup. There are **no license gates** for HA features.

## Topology

```text
[Client] → Caddy :8080 → storage-server (primary, writes)
                      ↘ storage-server-standby (STORAGE_READ_ONLY=true, :9001)
PostgreSQL primary ──streaming replication──► PostgreSQL standby
```

## Compose (lab)

```bash
docker compose -f docker-compose.yml -f docker-compose.ha.yml \
  --profile postgres --profile ha-standby up -d --build
```

| Service | Role |
|---------|------|
| `postgres` | Metadata primary |
| `postgres-standby` | Metadata replica (`--profile ha-postgres`) |
| `storage-server` | Primary API |
| `storage-server-standby` | DR read path (`STORAGE_READ_ONLY=true`) |

Set `STORAGE_POSTGRES_READ_REPLICA_DSN` on the primary to route list queries to the standby.

## Failover (manual)

1. Run `scripts/postgres-failover.ps1` or `scripts/postgres-failover.sh` (promote standby, health wait).
2. Update `STORAGE_POSTGRES_DSN` to the new primary.
3. Quarterly: `scripts/dr-drill.ps1`.

## Kubernetes (Helm)

```bash
helm upgrade --install datasafe ./deploy/helm/datasafe \
  -f deploy/helm/datasafe/values-ha.yaml
```

Deploys primary + read-only standby `Deployment` and Postgres. See [scaling.md](./scaling.md).

## Backup

- **Metadata:** `pg_dump` from primary or standby (prefer standby for consistent snapshots).
- **Objects:** filesystem snapshot of `STORAGE_DATA_DIR` or Gateway replication to external S3.
- **Restore:** restore Postgres, restore object volume, point DSN, verify `/healthz`.

## Verification

```bash
curl -s http://localhost:8080/healthz | jq .
powershell -File scripts/dr-drill.ps1
powershell -File scripts/federation-3node-demo.ps1
```
