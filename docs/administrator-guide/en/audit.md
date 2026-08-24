English | **[Русский](../ru/audit.md)**

# Audit and activity log

![Activity](../../images/screenshots/activity.png)

DataSafeS3 records administrative and data-plane actions in the **Activity** log.

## Events

- User/bucket/object CRUD
- Settings changes, login events
- Share link creation
- Gateway replication triggers

## Console

**Administration → Activity** — filter by action, user, resource; **Export CSV / JSON** downloads the filtered trail (admin-only). The export is itself logged as `activity_exported`.

Object Lock / retention changes and blocked deletes appear as `object_lock_changed`, `object_retention_set`, `versioning_changed`, `object_delete_blocked` (Admin JSON delete and S3 SigV4 `DeleteObject`).

## API

```http
GET /api/v1/activity?limit=100
GET /api/v1/activity/export?format=csv&period=30d&bucket=backups
```

Storage inventory (CSV listing with Lock fields):

```http
POST /api/v1/inventory/jobs
GET  /api/v1/inventory/jobs/{id}
GET  /api/v1/inventory/jobs/{id}/download
```

Operator checklist: [Governance evidence pack](../../use-cases/en/governance-evidence.md).

## Activity retention

`STORAGE_ACTIVITY_RETENTION_DAYS` (default 90; `0` disables purge). Hourly GC on Bolt and Postgres. Not a WORM journal — keep exported copies if policy requires longer retention.

## External logging

Duplicate structured JSON logs to Syslog, Loki, Elasticsearch, or Webhook — **Settings → External logging**.

See [operations guide — monitoring](../../operations-guide/en/monitoring.md).
