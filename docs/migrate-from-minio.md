# Move data from MinIO

Two ways to leave a MinIO bucket. Copy the objects with a client, or use the built-in copier when you also need versions, Object Lock, and IAM.

The client copy is the one most people run first. The built-in copier is [docs/operations-guide/en/migrate-from-minio.md](operations-guide/en/migrate-from-minio.md).

## mc mirror

```bash
mc alias set src http://minio:9000 MINIO_ROOT_USER MINIO_ROOT_PASSWORD
mc alias set dst http://datasafe:9000 ACCESS_KEY SECRET_KEY --api s3v4 --path on
mc mb dst/photos
mc mirror --overwrite src/photos dst/photos
```

`mc rm --recursive` against DataSafeS3 uses DeleteObjects. That works on 1.7.0.

## rclone sync

```bash
export RCLONE_CONFIG_SRC_TYPE=s3
export RCLONE_CONFIG_SRC_PROVIDER=Minio
export RCLONE_CONFIG_SRC_ENDPOINT=http://minio:9000
export RCLONE_CONFIG_SRC_ACCESS_KEY_ID=MINIO_ROOT_USER
export RCLONE_CONFIG_SRC_SECRET_ACCESS_KEY=MINIO_ROOT_PASSWORD

export RCLONE_CONFIG_DST_TYPE=s3
export RCLONE_CONFIG_DST_PROVIDER=Other
export RCLONE_CONFIG_DST_ENDPOINT=http://datasafe:9000
export RCLONE_CONFIG_DST_ACCESS_KEY_ID=ACCESS_KEY
export RCLONE_CONFIG_DST_SECRET_ACCESS_KEY=SECRET_KEY
export RCLONE_CONFIG_DST_FORCE_PATH_STYLE=true
export RCLONE_CONFIG_DST_REGION=us-east-1

rclone sync src:photos dst:photos
```

Path-style is required. Virtual-host buckets are not served.

## What the client copy does not bring across

| MinIO | After mc mirror or rclone sync |
|-------|--------------------------------|
| Current object bytes, names, and user metadata those tools send | Copied |
| Versions and delete markers | Not copied. Use `storage-cli migrate from-minio` |
| Object Lock, retention, legal hold | Not copied by these tools |
| Bucket policy and IAM users | Not copied. `storage-cli migrate from-minio iam` imports users, groups, and policies it can enforce. LDAP and OIDC are reported and not imported |
| Bucket encryption defaults | Not copied. Set encryption on the DataSafeS3 bucket after the copy |
| Lifecycle transition rules | Not stored. A prefix expiration in days is accepted. Other lifecycle rules are rejected |

## Settings that look the same

| MinIO | DataSafeS3 |
|-------|------------|
| `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` | `STORAGE_ACCESS_KEY` / `STORAGE_SECRET_KEY` for S3. The console admin is `STORAGE_ADMIN_USER` / `STORAGE_ADMIN_PASSWORD` |
| `MINIO_REGION` or the site region | `STORAGE_REGION` (default `us-east-1`) |
| Data directory | `STORAGE_DATA_DIR` (default `/data` in the container) |
| Distributed MinIO | One node uses Bolt. Several nodes share Postgres (`STORAGE_METADATA_BACKEND=postgres`, `STORAGE_HA_ENABLED=true`). They do not form a MinIO erasure set |
| Prometheus `/minio/v2/metrics/cluster` | `GET /metrics` with `STORAGE_METRICS_TOKEN` when that variable is set. Leave `STORAGE_DEV` unset or `false` |

Point aws-cli, mc, and rclone at `http://host:9000` with `--addressing-style path` (or mc `--path on`). The compatibility matrix is [compatibility.md](compatibility.md).
