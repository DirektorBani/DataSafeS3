# scripts/migrate — MinIO cutover helpers

Smoke verification after syncing a bucket from a MinIO-compatible source to DataSafeS3.

See ops guide: [migrate-from-minio.md](../../docs/operations-guide/en/migrate-from-minio.md) (EN) · [RU](../../docs/operations-guide/ru/migrate-from-minio.md).

## Autotest suite

```powershell
powershell -NoProfile -File scripts/migrate/test-minio-migration-kit.ps1
```

Checks: kit artifacts, EN/RU honesty markers, CHANGELOG `[1.1.1]`, `go test` for migrate/events/inventory, DryRun smoke.  
Optional live: `$env:MIGRATE_LIVE=1` plus `SOURCE_*` / `DEST_*` / `MIGRATE_BUCKET`.

## minio-cutover-smoke.ps1

Compares object **counts** (ListObjectsV2 via AWS CLI) and samples key sizes between source and destination.

**Requirements for live mode:** AWS CLI v2 on PATH.

**Secrets:** pass `-SourceSecret` / `-DestSecret` or set env `SOURCE_SECRET` / `DEST_SECRET`. The script never prints secret values.

```powershell
# Plan only
pwsh -File scripts/migrate/minio-cutover-smoke.ps1 -DryRun `
  -SourceEndpoint http://127.0.0.1:9001 `
  -DestEndpoint http://127.0.0.1:9000 `
  -Bucket demo

# Live
$env:SOURCE_SECRET = "<minio-secret>"
$env:DEST_SECRET = "<datasafe-secret>"
pwsh -File scripts/migrate/minio-cutover-smoke.ps1 `
  -SourceEndpoint http://127.0.0.1:9001 -SourceKey minioadmin `
  -DestEndpoint http://127.0.0.1:9000 -DestKey datasafe `
  -Bucket demo -SampleSize 20
```

| Param | Meaning |
|-------|---------|
| `-AllowCountDrift` | Exit 0 even if counts differ (prints WARN) |
| `-SampleSize` | Number of keys to compare (default 20) |
| `-DryRun` | Print planned checks; exit 0 |

Exit code **0** = PASS (or DryRun). Non-zero = FAIL.
