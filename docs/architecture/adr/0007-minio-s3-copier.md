# ADR 0007: MinIO S3 copier

**Status:** Accepted  
**Date:** 2026-10-14  
**Release:** v1.6.0  

## Context

Operators asked for `storage-cli migrate from-minio` that copies versions, delete markers, Object Lock, tags, and user metadata, then writes a PASS/FAIL evidence pack. ADR 0001 kept object bytes on rclone so the product would not claim a MinIO importer.

## Decision

Add a resumable S3 copier in `internal/migrate`, driven by `storage-cli migrate from-minio`.

- State is JSONL in `--state` (default `./.minio-migrate-state`). No new database driver.
- A version is skipped when the state file records it, when target metadata `x-amz-meta-datasafe-source-version` matches the source version id, or when ETag and size match and `--sse` is not set. SSE changes the ETag, so the metadata key is the idempotency check in that case.
- Versions of one key are copied oldest first. The current version is last, so the target head matches the source.
- Bucket policy statements this server cannot evaluate are omitted and warned. They are not applied.
- HTTP 503 `not cluster leader` is retried. Several `--target` values are resolved with `GET /healthz` `is_leader`.
- The report is `summary.md`, `evidence.json` with top-level `verdicts`, and `evidence.zip`.
- **Settings → Migrate** prints the command with `$SOURCE_ACCESS_KEY` and the other key variables. The browser does not receive or store source credentials. It can open a local `evidence.json`.

The checklist, rclone cookbook, and smoke script from ADR 0001 stay. IAM import is `storage-cli migrate from-minio iam` in the same release: users, groups, service accounts, and policies this server can enforce. LDAP and OIDC are reported and not imported. MinIO secrets are not copied.

## Consequences

- HEAD and GET echo `x-amz-meta-*`. Delete markers and multipart uploads keep that metadata. `ListObjectVersions` marks the current version from the latest index and emits delete markers. A versioned multipart complete moves the assembled file aside before writing `key/versions/<id>`.
- These paths run on the request. An idle single node does not start a copier, a Vault client, or a new metric.
- DataSafe serves one versions page (the copier asks for 100000 keys). MinIO pages are followed.
- A 10,000-object run, a multipart object over 100MB, and a 3-node leader failure remain operator acceptance on MinIO. CI covers two DataSafe processes and, when the image is available, one MinIO container with a small object.
