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

**Administration → Activity** — filter by action, user, resource.

## API

```http
GET /api/v1/activity?limit=100
```

## External logging

Duplicate structured JSON logs to Syslog, Loki, Elasticsearch, or Webhook — **Settings → External logging**.

See [operations guide — monitoring](../../operations-guide/en/monitoring.md).
