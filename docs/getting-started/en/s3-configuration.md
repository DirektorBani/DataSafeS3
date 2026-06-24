English | **[Русский](../ru/s3-configuration.md)**

# External S3 configuration

Optional step in the setup wizard or later via **Admin → Gateway**.

## Use cases

- Async replication of buckets to external S3-compatible storage
- Hybrid storage (hot local + cold remote)
- Disaster recovery target

## Setup wizard fields

| Field | Example |
|-------|---------|
| Endpoint | `http://minio:9000` |
| Access key | your key |
| Secret key | your secret |
| Bucket | `datasafe-backup` |
| Region | `us-east-1` |
| Use SSL | toggle |

## Test connection

```http
POST /api/v1/setup/s3/test
```

## After setup

Manage replication rules in **Gateway** page. See [Administrator guide — replication](../../administrator-guide/en/replication.md) and [user guide Gateway chapter](../../en/user-guide/06-gateway-and-minio.md).
