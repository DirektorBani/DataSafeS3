# Partner integration cookbook

English | **[Русский](../ru/partner-cookbook.md)**

Recipes for backup, Kubernetes, and SIEM integrations with DataSafeS3 Community Edition.

## Velero (Kubernetes backup)

1. Install Velero with an S3-compatible plugin pointing at DataSafeS3:

```yaml
apiVersion: velero.io/v1
kind: BackupStorageLocation
metadata:
  name: datasafe
spec:
  provider: aws
  objectStorage:
    bucket: velero-backups
  config:
    region: us-east-1
    s3Url: https://datasafe.example.com
    s3ForcePathStyle: "true"
```

2. Create access keys in DataSafeS3 Admin → Users.
3. Store keys in a Kubernetes `Secret` referenced by Velero credentials.

## restic (off-host backup)

```bash
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export RESTIC_REPOSITORY=s3:https://datasafe.example.com/my-backup-bucket
restic backup /data/to/archive
```

Use path-style endpoint and `--option s3.region=us-east-1` if required by your restic version.

## SIEM (webhook / NATS)

| Sink | Configuration |
|------|----------------|
| Webhook | Admin → Webhooks, subscribe to `object.created` / `share.downloaded` |
| NATS | `STORAGE_NATS_URL=nats://nats:4222`, compose profile `nats` |

Example NATS compose:

```bash
docker compose -f docker-compose.yml -f docker-compose.ha.yml --profile nats up -d nats
```

## STS scoped upload (integrators)

Call `POST /api/v1/sts/assume-role` with the **same user context** you want scoped S3 access for (API token `ds_*` or session JWT). Returned credentials are **bound to that user** — not a shared admin key.

```bash
curl -s -X POST http://localhost:8080/api/v1/sts/assume-role \
  -H "Authorization: Bearer $DS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"role_arn":"arn:datasafe:role/uploader","role_session_name":"partner-job"}'
```

Use returned `session_token` with `X-Amz-Security-Token` on S3 SigV4 requests.

## SDK examples

See `examples/go`, `examples/python`, `examples/js`, and `examples/extension-hook/`.
