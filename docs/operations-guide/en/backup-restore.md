English | **[Русский](../ru/backup-restore.md)**

# Backup and restore

## What to back up

| Asset | Path / method |
|-------|---------------|
| Object data | `STORAGE_DATA_DIR/objects/` |
| BoltDB metadata | `STORAGE_DATA_DIR/metadata.db` |
| PostgreSQL metadata | `pg_dump` of `STORAGE_POSTGRES_DB` |
| Configuration | `.env`, Kubernetes Secrets |

## Backup procedure

```bash
# Stop writes (optional, for consistent snapshot)
docker compose stop storage-server

# Copy data volume
tar czf datasafe-backup-$(date +%F).tar.gz ./data/

# PostgreSQL
docker exec datasafe-postgres-1 pg_dump -U datasafe datasafe > metadata.sql

docker compose start storage-server
```

## Restore

1. Deploy fresh DataSafeS3 with same `STORAGE_*` config
2. Restore `objects/` and `metadata.db` OR import PostgreSQL dump
3. Start `storage-server` and verify buckets/objects

## Gateway replication

Use [Gateway replication](../../administrator-guide/en/replication.md) as continuous off-site copy to external S3.

## SSE master key rotation {#sse-master-key-rotation}

When `STORAGE_SSE_MASTER_KEY` is set, server-side encryption (SSE-S3) derives per-object keys from this master secret.

1. **Prepare** — ensure you can restore from backup; schedule a maintenance window.
2. **Re-encrypt** — there is no automatic in-place re-key in Community Edition today. To rotate:
   - Deploy a new key on a **new** instance or after full backup.
   - Copy objects out and back in (or restore from unencrypted backup) so they are written with the new key.
3. **Update env** — set the new `STORAGE_SSE_MASTER_KEY` in `.env`, Kubernetes Secrets, or Helm `storageServer.config.sseMasterKey`.
4. **Verify** — upload a test object and confirm GET succeeds; check audit/activity logs.
5. **Retire old key** — securely delete the previous secret from secret stores after validation.

For production, combine rotation with [backup procedure](#backup-procedure) and `STORAGE_STRICT_SECRETS=true` for default credential checks at startup.
