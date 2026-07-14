English | **[Русский](../ru/migrate-from-minio.md)**

# Migrate from MinIO to DataSafeS3

Operator runbook for moving object data from a **MinIO-compatible** S3 endpoint to **DataSafeS3** Community Edition.

> **Honesty:** This is not a drop-in binary replacement and not a claim of 100% MinIO API parity. Object bytes move with standard S3 sync tools. MinIO IAM users, groups, and server-side policies are **not** imported automatically — remap them to DataSafe users, teams, tenants, and bucket policies.

Architecture: [ADR 0001](../../architecture/adr/0001-migration-kit.md) · Checklist source: `internal/migrate`.

---

## 1. When to use this guide

| Scenario | Approach |
|----------|----------|
| Existing MinIO (or MinIO-compatible) buckets with data | **Parallel cutover** (recommended) |
| Temporary hybrid while apps move | Optional [Gateway](../../en/context/gateway.md) bridge |
| Empty target / greenfield | Skip sync — create buckets and keys only |

## 2. Prerequisites

- DataSafeS3 running; `GET /healthz` returns OK
- **PostgreSQL** metadata recommended for production ([first run](../../getting-started/en/first-run.md))
- Network: operator host reaches **source** MinIO and **target** DataSafe S3 ports
- Tools: [rclone](https://rclone.org/) **or** AWS CLI v2
- On DataSafe: buckets created, S3 access keys issued (Admin → Keys)

## 3. What transfers / what does not

| Transfers via S3 sync | Does **not** transfer automatically |
|----------------------|-------------------------------------|
| Object bytes | MinIO IAM users / groups |
| Common user metadata headers preserved by S3 PUT | MinIO policy JSON as-is (remap principals) |
| Multipart-completed objects (as final objects) | MinIO server config, console bookmarks |
| | Object Lock / versioning **server config** — re-enable on DataSafe if required |

## 4. Remapping cheat sheet

| MinIO | DataSafeS3 |
|-------|------------|
| Access Key / Secret | S3 access keys (Admin → Keys) |
| Bucket | Bucket (same name recommended) |
| Bucket policy JSON | Bucket policy UI / JSON (remap principals) |
| Users / groups | Users + [Teams](../../administrator-guide/en/teams.md) + Tenants |
| Server-side replication | [Gateway](../../administrator-guide/en/replication.md) or [Trusted clusters](./trusted-clusters.md) (DataSafe↔DataSafe) |
| Console | DataSafe web console |

## 5. rclone configuration

Example remotes (path-style; adjust endpoints):

```ini
[minio]
type = s3
provider = Minio
env_auth = false
access_key_id = SOURCE_ACCESS_KEY
secret_access_key = SOURCE_SECRET_KEY
endpoint = http://minio.example.com:9000
acl = private

[datasafe]
type = s3
provider = Other
env_auth = false
access_key_id = DATASAFE_ACCESS_KEY
secret_access_key = DATASAFE_SECRET_KEY
endpoint = http://datasafe.example.com:9000
acl = private
force_path_style = true
```

Copy-paste file: [examples/rclone-minio-to-datasafe.conf](./examples/rclone-minio-to-datasafe.conf).

### Sync

```bash
rclone sync minio:my-bucket datasafe:my-bucket --checksum --progress
```

Repeat per bucket. For a dry inventory: `rclone size minio:my-bucket` vs `rclone size datasafe:my-bucket`.

## 6. aws-cli alternative

```bash
aws --endpoint-url http://minio.example.com:9000 s3 sync s3://my-bucket /tmp/my-bucket
aws --endpoint-url http://datasafe.example.com:9000 s3 sync /tmp/my-bucket s3://my-bucket
```

Or dual-endpoint sync if your tooling supports it; prefer rclone for two remotes.

## 7. Cutover checklist

Printable copy:

```bash
go run ./cmd/storage-cli migrate checklist minio
# or after build: storage-cli migrate checklist
```

Same text: `internal/migrate.ChecklistMarkdown()` and steps below:

1. Freeze or pause writers on MinIO (maintenance window)
2. Final `rclone sync … --checksum`
3. Run smoke: `pwsh -File scripts/migrate/minio-cutover-smoke.ps1` (see [scripts/migrate/README.md](../../../scripts/migrate/README.md))
4. Point applications / DNS to DataSafe endpoint
5. Unfreeze writers **only** against DataSafe
6. Keep MinIO read-only for a rollback window (e.g. 7–14 days)
7. Do **not** delete source until smoke PASS and apps are stable

## 8. Verify (smoke script)

```powershell
pwsh -File scripts/migrate/minio-cutover-smoke.ps1 -DryRun `
  -SourceEndpoint http://minio:9000 -DestEndpoint http://127.0.0.1:9000 -Bucket my-bucket

# Live (requires AWS CLI; secrets via env — never log them):
$env:SOURCE_SECRET = "…"
$env:DEST_SECRET = "…"
pwsh -File scripts/migrate/minio-cutover-smoke.ps1 `
  -SourceEndpoint http://minio:9000 -SourceKey AKIA… `
  -DestEndpoint http://127.0.0.1:9000 -DestKey datasafe `
  -Bucket my-bucket -SampleSize 20
```

## 9. Post-cutover

- Re-enable **Object Lock** / **versioning** on DataSafe buckets if your backup jobs require immutability (source server flags are not a substitute). See [backup use-case](../../use-cases/en/backup-storage.md); fuller immutable path is planned (ADR [0003](../../architecture/adr/0003-immutable-backup-path.md)).
- Wire LDAP/OIDC if you used MinIO identity plugins — [onboarding](../../getting-started/en/onboarding.md).
- Optional off-site copy: [Gateway replication](../../administrator-guide/en/replication.md).

## 10. Rollback

1. Stop writers to DataSafe (or dual-write carefully).
2. Point apps back to MinIO endpoint.
3. Treat DataSafe copy as stale until a new sync.

## 11. FAQ

**Q: Can I import MinIO users automatically?**  
No. Create DataSafe users/teams/tenants and issue new S3 keys.

**Q: Is Gateway required?**  
No. Prefer parallel sync + cutover. Gateway is optional for hybrid/DR to an external S3 endpoint.

**Q: Trusted clusters?**  
Use when **both** sites run DataSafeS3 — [trusted-clusters.md](./trusted-clusters.md). For MinIO as source during migration, use rclone/aws sync (this guide).
