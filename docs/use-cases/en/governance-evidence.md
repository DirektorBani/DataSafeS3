English | **[Русский](../ru/governance-evidence.md)**

# Governance evidence pack (operator checklist)

Assemble **operator evidence** for RFP / InfoSec review: what is stored, who did what, and what Object Lock currently protects.

> This is **not** certified compliance (ISO/SOC as a product), **not** AWS S3 Inventory + Athena, and **not** a WORM journal for the activity trail itself.

## Prerequisites

- Admin role in the console (or Admin API JWT / API token with admin scope)
- A control bucket with Object Lock / retention configured (see [immutable backup](immutable-backup.md))
- Optional: `STORAGE_ACTIVITY_RETENTION_DAYS` (default **90**; set `0` to disable GC of old activity rows)

## Checklist (one working day)

1. **Enable Lock** on the control bucket (console → bucket → Settings, or Admin `PUT /api/v1/settings/buckets/{name}`).
2. **Upload** a test object; optionally set per-object retention via Admin API.
3. **Attempt delete** — expect HTTP 403 and an Activity row `object_delete_blocked` (Admin JSON delete **and** S3 SigV4 `DeleteObject`).
4. **Storage inventory (CSV)** — bucket Settings → *Storage inventory*, or:
   ```http
   POST /api/v1/inventory/jobs
   {"bucket":"backups","prefix":"prod/","format":"csv"}
   ```
   Then `GET /api/v1/inventory/jobs/{id}/download`. Optional `dest_bucket` / `dest_key` writes the same CSV as an object. Cron/schedule is **501** in this release (manual only).
5. **Activity export** — Administration → Activity → Export CSV/JSON (same filters as the UI), or:
   ```http
   GET /api/v1/activity/export?format=csv&period=30d&bucket=backups
   ```
   The export itself is logged as `activity_exported`.
6. **Package folder** for the auditor: inventory CSV + activity CSV/JSON + Lock settings screenshot or `GET` settings JSON.

### Helper script (Windows PowerShell)

```powershell
# Host PowerShell — against a running storage-server
$BaseUrl = "http://127.0.0.1:9000"
$tok = (Invoke-RestMethod -Method POST "$BaseUrl/api/v1/admin/login" `
  -ContentType "application/json" -Body '{"username":"admin","password":"admin"}').token
.\scripts\collect-evidence-pack.ps1 -BaseUrl $BaseUrl -Token $tok -Bucket backups -Prefix "prod/" -Period 30d
```

Creates `evidence-pack-<timestamp>/` plus a `.zip` (inventory + activity + bucket settings + README).

## CSV columns (inventory)

`bucket`, `key`, `size`, `last_modified`, `storage_class`, `version_id`, `object_lock_enabled`, `bucket_retention_days`, `retention_mode`, `retention_until`, `legal_hold`

Hard cap: **100 000** objects per job (`truncated: true` if more). Jobs are in-memory until process restart; dest-bucket copies remain on storage.

## Activity trail retention

Hourly GC (plus one run at process start) deletes activity older than `STORAGE_ACTIVITY_RETENTION_DAYS` (Bolt and Postgres). Default 90 days. This is disk hygiene, **not** compliance-grade immutable audit storage — ship copies of exports to your SIEM if you need long retention.

## Related

- [Immutable backup](immutable-backup.md)
- [Audit / Activity](../../administrator-guide/en/audit.md)
- ADR-0004 Inventory jobs
