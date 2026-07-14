English | **[Русский](../ru/backup-storage.md)**

# Backup repository

## Problem

Backup tools (Veeam, restic, Velero, custom scripts) need a durable S3-compatible target on infrastructure the organization controls.

## Solution

Use DataSafeS3 as the primary backup landing zone:

```mermaid
flowchart LR
  backup[Backup software]
  ds[DataSafeS3 S3]
  gw[Gateway optional]
  remote[External S3 site]
  backup -->|S3 API| ds
  ds -->|async replication| gw --> remote
```

1. Deploy with PostgreSQL metadata for production ([first run](../../getting-started/en/first-run.md))
2. Create dedicated buckets per workload (e.g. `backups-database`, `backups-k8s`)
3. Issue S3 access keys per backup job (least privilege)
4. Optional: [Gateway replication](../../administrator-guide/en/replication.md) for off-site copies
5. [Lifecycle rules](../../administrator-guide/en/lifecycle.md) to expire old restore points

## Result

Predictable, self-hosted backup target with optional geo-redundant copies via Gateway — under your retention and access policies.

## Migrating an existing MinIO backup target

If backups already land on MinIO (or another S3-compatible store), cut over buckets with the [MinIO → DataSafeS3 migration kit](../../operations-guide/en/migrate-from-minio.md), then re-check Object Lock / versioning on the DataSafe side.

## Verified script

Smoke-test bucket create + object round-trip against a running stack:

```powershell
.\scripts\reference-arch\backup-restore.ps1
# optional: -BaseUrl http://127.0.0.1:8080
```

Expect `PASS reference-arch backup-restore smoke`. See also [backup & restore operations guide](../../operations-guide/en/backup-restore.md).
