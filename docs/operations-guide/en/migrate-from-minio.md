English | **[Русский](../ru/migrate-from-minio.md)**

# Migrate from MinIO to DataSafeS3

Operator runbook for moving object data from a **MinIO-compatible** S3 endpoint to **DataSafeS3** Community Edition.

> **Honesty:** This is not a drop-in binary replacement and not a claim of 100% MinIO API parity. Object bytes move with standard S3 sync tools. `storage-cli migrate from-minio iam` imports builtin users, groups, and canned policies. MinIO user secrets are not available and are not copied. LDAP and OIDC identities are warnings with a recommendation, not accounts.

Architecture: [ADR 0001](../../architecture/adr/0001-migration-kit.md) · [ADR 0007](../../architecture/adr/0007-minio-s3-copier.md) · Checklist source: `internal/migrate`.

## Embedded copier

`storage-cli migrate from-minio` copies buckets over the S3 API. rclone and the checklist below remain the DIY path. IAM is a separate command, below.

The copier writes, in order: bucket, versioning, Object Lock configuration, lifecycle, and bucket policy. Lifecycle keeps a prefix expiration in days (`Filter/Prefix`, `Filter/And/Prefix`, or the legacy `Prefix`). A transition, noncurrent action, tag filter, or date expiration is not stored and the run fails; the rule is not widened to the whole bucket. A policy condition other than `aws:SourceIp` or `aws:SecureTransport` is omitted and named in the report. Object versions and delete markers are copied in chronological order, so a delete marker stays between the versions it separates. Tags, user metadata, content-type, retention mode and date, and legal hold are copied with the version.

Resume state is JSONL in `--state` (default `./.minio-migrate-state`), flushed after each object. A later run skips a version when that file records it, when the target metadata `x-amz-meta-datasafe-source-version` equals the source version id, or, when `--sse` is not set, when ETag and size already match. Ctrl+C is safe. Repeat the same command to continue. The same source version is not written twice.

A target that answers HTTP 503 `not cluster leader` is retried with backoff. Repeat `--target` for each node; `GET /healthz` selects the one with `is_leader: true`.

`--sse AES256` or `--sse aws:kms --kms-key …` is sent on the target PUT. Source SSE-KMS is read through the S3 API. Source bucket encryption is not copied. An empty source configuration does not turn off encryption the target bucket already has. `CreateBucket` on a bucket the caller already owns is HTTP 409 and does not change that bucket. Keys may be flags, `SOURCE_ACCESS_KEY` / `SOURCE_SECRET_KEY` / `TARGET_ACCESS_KEY` / `TARGET_SECRET_KEY`, or `--source-secret-file` and `--target-secret-file`. The report directory (`--report`, default `./minio-migrate-report`) contains `summary.md`, `evidence.json` (`verdicts` are PASS or FAIL), and `evidence.zip`. Verdicts are object count, bytes, delete-marker order, ETag (a multipart ETag, or a single-part ETag that is not the plaintext MD5 such as SSE-KMS, is checked with SHA-256 of the downloaded object, including when `x-amz-meta-datasafe-source-version` is missing), Object Lock mode, retain-until date, and legal hold, a SHA-256 sample (`--verify-sample` checks at least one object; `--verify-full` counts a missing object as a mismatch), and `encryption`. The `encryption` verdict fails when a target bucket that already had default encryption loses it, or a copied object is stored without `x-amz-server-side-encryption`. The target bucket is listed once. A follower forwards writes to the leader.

DataSafe returns one `ListObjectVersions` page. The copier asks for 100000 keys. MinIO pages of 1000 are followed.

```bash
storage-cli migrate from-minio \
  --source http://minio.example:9000 \
  --source-key "$SOURCE_ACCESS_KEY" \
  --source-secret "$SOURCE_SECRET_KEY" \
  --target http://datasafe.example:9000 \
  --target-key "$TARGET_ACCESS_KEY" \
  --target-secret "$TARGET_SECRET_KEY" \
  --buckets my-bucket \
  --verify-sample 10
```

**Settings → Migrate** in the console prints this command and the IAM command, and can open a local `evidence.json`. It does not send or store source credentials.

## IAM

`storage-cli migrate from-minio iam` reads the MinIO admin API (`/minio/admin/v3/`, the same calls as `mc admin`) with the source admin keys. Groups are listed with `GET /groups` and described with `GET /group?group=`. It creates DataSafe users, teams, and bucket policies. `--tenant` sets the tenant id. A bucket that does not already exist is named in the report and its policy is not stored.

Imported S3 rights are equal to MinIO or stricter. An Allow that this server cannot enforce exactly is dropped and named as WARN. A Deny whose condition cannot be enforced is stored without that condition, so it denies unconditionally, and the report says so. `aws:SourceIp` and `aws:SecureTransport` stay on the statement and are evaluated. A policy for every bucket (`*`) is not copied. A service account with an inline policy does not receive parent allows that the inline policy does not grant. A disabled or already expired service account does not get a key. An expiration in the future is stored on the new key. A user whose policy does not allow `s3:ListAllMyBuckets` cannot list every bucket. The import does not write bucket grants, so an existing grant is left in place. A repeated run merges bucket policy statements and does not remove statements already stored.

MinIO does not return a usable user secret for this import. The command generates a password and an access key, and writes them to `--keys-file` (default `./minio-iam-keys.json`). The file mode is 0600. On Windows the command sets an owner-only ACL and records a WARN if that ACL cannot be set. A second run keeps the secrets already in the file and appends keys that were missing; it does not replace the file with empty passwords. `--keep-access-keys FILE` imports `username`, `access_key`, and `secret_key` from that operator file instead of generating the key. A service account secret is not available; the report says so and, when the account is enabled and unexpired, a new key is issued for the parent user. LDAP and OIDC users are WARN only: configure those providers on DataSafe. Group policies are expanded to member access keys only while the group is enabled, because a DataSafe bucket policy has no group principal. The report lists each group's status and whether its policy was applied, and each service account's disabled state, expiration, and inline-policy narrowing.

Several `--target` values are checked with `GET /healthz`. Writes that return HTTP 503 `not cluster leader` are retried against the leader. The target login is `--target-admin-user` and `--target-admin-password` (or `TARGET_ADMIN_USER` / `TARGET_ADMIN_PASSWORD`). The report directory (`--report`, default `./minio-iam-report`) contains `summary.md` and `evidence.json`. Verdicts are users, groups, policies, and access. Access is a signed `ListBucket` with the new key.

```bash
storage-cli migrate from-minio iam \
  --source http://minio.example:9000 \
  --source-key "$SOURCE_ACCESS_KEY" \
  --source-secret "$SOURCE_SECRET_KEY" \
  --target http://datasafe.example:9000 \
  --target-admin-user "$TARGET_ADMIN_USER" \
  --target-admin-password "$TARGET_ADMIN_PASSWORD" \
  --keys-file ./minio-iam-keys.json \
  --report ./minio-iam-report
```

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

- Re-enable **Object Lock** / **versioning** on DataSafe buckets if your backup jobs require immutability (source server flags are not a substitute). Follow the [immutable backup](../../use-cases/en/immutable-backup.md) golden path ([ADR 0003](../../architecture/adr/0003-immutable-backup-path.md)).
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
