# Access model

Who can read, write, share, and administer a bucket. The matrix below is generated from `internal/auth/access_matrix.go`. A test fails when this section and the Russian guide diverge from that table.

Cells:

- `allow` — the role may perform the action.
- `deny` — the role may not.
- `write-grant` — only when the principal also has a write grant on that bucket. Operators do not receive write on every bucket.
- `compat-flag` — only while `STORAGE_OPERATOR_WRITE_ALL=true`. That variable is deprecated and is removed in 1.7.0. The process logs a warning at startup, writes Activity `operator_write_all_enabled`, and the console shows a banner to global administrators.

Bucket admin is the bucket owner, a tenant administrator of the bucket's tenant, a global administrator, or a grant with level `admin`. Write and read grants do not include purge, COMPLIANCE retention, grant changes, or sharing policy.

Anonymous reads are allowed only for a single unambiguous public bucket (header `X-DataSafe-Tenant` or query `tenant` when names collide) and only when Block Public Access is off at every level. `public-read` is stored as a bucket policy with `Principal: "*"`.

Presigned URLs are signed with a temporary access key owned by the caller. The root access key cannot presign. DELETE cannot be presigned. The TTL cannot exceed 7 days or the effective `max_presign_ttl`.

STS `AssumeRole` rejects a non-empty `RoleArn`. The session inherits the caller's permissions. The returned ARN is the caller's user ARN.

`POST /api/v1/clusters/{id}/rotate` and `/revoke-notify` require a client certificate this cluster issued to an active trusted peer (TLS client certificate, or `X-DataSafe-Cluster-Cert` plus `X-DataSafe-Cluster-Sig` on plain HTTP). A request without that proof is `401`.

<!-- access-matrix:begin -->
| Action | Global admin | Operator | Tenant admin | Bucket admin | Write | Read | Anonymous |
| --- | --- | --- | --- | --- | --- | --- | --- |
| presign GET | allow | allow | allow | allow | allow | allow | deny |
| presign PUT | allow | write-grant | allow | allow | allow | deny | deny |
| presign DELETE | deny | deny | deny | deny | deny | deny | deny |
| create public-read bucket | allow | deny | allow | deny | deny | deny | deny |
| set public-read after create | allow | deny | allow | allow | deny | deny | deny |
| anonymous GET when public and not blocked | allow | allow | allow | allow | allow | allow | allow |
| anonymous GET when block public access | deny | deny | deny | deny | deny | deny | deny |
| list own share links | allow | allow | allow | allow | allow | deny | deny |
| list every share link | allow | deny | allow | allow | deny | deny | deny |
| create share link | allow | write-grant | allow | allow | allow | deny | deny |
| revoke own share link | allow | write-grant | allow | allow | allow | deny | deny |
| revoke another user's share link | allow | deny | allow | allow | deny | deny | deny |
| revoke all share links | allow | deny | allow | allow | deny | deny | deny |
| change bucket sharing policy | allow | deny | allow | allow | deny | deny | deny |
| change tenant sharing policy | allow | deny | allow | deny | deny | deny | deny |
| change global sharing policy | allow | deny | deny | deny | deny | deny | deny |
| legal hold | allow | write-grant | allow | allow | allow | deny | deny |
| retention GOVERNANCE | allow | write-grant | allow | allow | allow | deny | deny |
| retention COMPLIANCE | allow | deny | allow | allow | deny | deny | deny |
| change storage class | allow | write-grant | allow | allow | allow | deny | deny |
| restore from trash | allow | write-grant | allow | allow | allow | deny | deny |
| purge trash | allow | deny | allow | allow | deny | deny | deny |
| change bucket grants | allow | deny | allow | allow | deny | deny | deny |
| delete bucket | allow | compat-flag | allow | allow | deny | deny | deny |
| operator write without a grant | deny | deny | deny | deny | deny | deny | deny |
| cluster rotate or revoke-notify without a peer certificate | deny | deny | deny | deny | deny | deny | deny |
| STS RoleArn echoed unchecked | deny | deny | deny | deny | deny | deny | deny |
<!-- access-matrix:end -->
