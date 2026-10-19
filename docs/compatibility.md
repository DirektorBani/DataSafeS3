# S3 compatibility

DataSafeS3 speaks the S3 HTTP API that backup tools and artifact stores use on a single node: path-style URLs, SigV4, and Bolt metadata with no extra database. It is not a complete implementation of every Amazon S3 API.

The automated check is `go test ./internal/api/s3/ -run TestS3ClientCompatibility`. It runs whichever of `aws`, `mc`, `rclone`, and `restic` are on `PATH`. CI sets `S3_COMPAT_REQUIRE=aws,mc,rclone,restic` so a missing client fails the job. `TestAWSSDKDeleteObjects` calls `DeleteObjects` through aws-sdk-go-v2.

## Clients exercised

| Client | What the test does | Result |
|--------|--------------------|--------|
| aws-cli v2 | Create/head bucket, put/get/head, list-objects-v2, copy, presign, multipart, versioning, `s3 rm --recursive` | Expected to pass |
| MinIO Client `mc` | `mb`, `cp`, `ls`, `rm --recursive` | Expected to pass |
| rclone | `copyto`, `ls`, `delete` against provider `Other` | Expected to pass |
| restic | `init`, `backup`, `snapshots`, `forget --prune` | Expected to pass |
| aws-sdk-go-v2 | `CreateBucket`, `PutObject`, `DeleteObjects`, `GetObject` | Unit test, always runs |

`s3 rm --recursive`, `mc rm --recursive`, rclone deletes, and restic prune depend on `POST /{bucket}?delete` (DeleteObjects). That call was HTTP 405 before 1.7.0.

## Operations

| Operation | Support |
|-----------|---------|
| ListBuckets, CreateBucket, DeleteBucket, HeadBucket, GetBucketLocation | Yes |
| PutObject, GetObject, HeadObject, DeleteObject, CopyObject | Yes |
| ListObjects and ListObjectsV2 (max 1000, continuation) | Yes |
| Multipart upload, upload part, complete, abort, list uploads | Yes |
| DeleteObjects (up to 1000 keys, Quiet, per-key errors, Content-MD5 or `x-amz-checksum-*`) | Yes, since 1.7.0 |
| Bucket versioning and delete markers | Yes |
| List object versions | Yes |
| Object tagging | Yes |
| Bucket policy (a subset of IAM statements, including Deny and `aws:SourceIp`) | Yes |
| Presigned GET | Yes |
| Presigned PUT | Partial. `Content-Type` may be unsigned. Presigned DELETE is rejected |
| Bucket and object encryption headers (`AES256`, `aws:kms`) | Yes. `aws:kms` uses Vault Transit only when that provider is configured; otherwise the local server key |
| Object Lock, retention, legal hold | Yes. `x-amz-bypass-governance-retention: true` plus `s3:BypassGovernanceRetention` skips GOVERNANCE. COMPLIANCE and legal hold are not skipped |
| Lifecycle expiration by prefix | Yes. Transition, noncurrent expiration, and tag filters are rejected |
| Bucket notification configuration | Stored and delivered through the outbox when a sink is configured |
| Server-side checksum trailers on aws-chunked uploads (CRC32, CRC32C, SHA-1, SHA-256, CRC64NVME) | Yes |
| Object ACLs, bucket ACLs, S3 website, CORS API, select, restore, replication configuration API, inventory configuration API, accelerate, requester pays, object lambda | No |
| Virtual-hosted bucket URLs (`bucket.host`) | No. Use path-style (`addressing_style = path`) |
| AWS chunked streaming uploads | Yes, decoded before they are stored |

A delete of a missing key in DeleteObjects is reported as deleted. A missing `versionId` is `NoSuchVersion` for that key. The HTTP status of a well-formed DeleteObjects call is 200 even when some keys fail.

## What is still different from Amazon S3 and MinIO

- One process owns the bytes on its disk. Extra pods share metadata through Postgres and copy object bytes to peers. That is not MinIO erasure coding across servers.
- ETag of a single PUT is MD5 of the stored body for a plaintext object. SSE and multipart ETags are not plaintext MD5.
- A GET of a local object does not re-check the bytes against the stored ETag. A copy accepted from a peer is checked.
- Bucket policy evaluation is the subset documented in the IAM migration notes, not the full IAM language.
