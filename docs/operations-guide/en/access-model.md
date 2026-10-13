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

## Cluster writes

Sharing and security changes are ordinary writes. With `STORAGE_HA_ENABLED=true` and Postgres metadata, a follower returns **503** `not cluster leader` and `Retry-After: 30`, the same rule as since v1.1. Send the write to the leader or to a load balancer that routes writes to the leader. This covers:

- `PUT /api/v1/settings/sharing`, `PUT /api/v1/tenants/{id}/sharing`, `PUT /api/v1/buckets/{bucket}/sharing`
- `POST /api/v1/buckets/{bucket}/shares` and `POST .../shares/revoke-all`
- `DELETE /api/v1/shares/{id}`
- presign, which creates a temporary access key
- `PUT /api/v1/settings/mfa-policy`, `POST /api/v1/auth/step-up`, `POST /api/v1/users/{id}/mfa/reset`, and `POST /api/v1/keys`

`GET /api/v1/public/share/{token}` and its download stay available on a follower. The first rejected request after the link expires, or after `max_downloads` is used up, emits `ShareExpired` or `ShareLimitReached` once (one Activity row, `share.expired` or `share.limit_reached`, and one outbox row). Later rejects of that link do not emit again. `ShareBlocked` is emitted on every download attempted while sharing is off. A follower still writes that outbox row when a webhook matches, even if this process has not registered the sink; the leader delivers it.

## Bucket-admin 2FA

`PUT /api/v1/settings/mfa-policy` (global administrator) stores the policy in `SystemConfig.MFA`. Every node reads it on the next request. `require_admin_mfa` remains and is the same switch as `apply_to_global_admins`.

Who is covered when `require_bucket_admin_mfa` is on: a user with grant level `admin` on at least one bucket, including the owner, plus tenant administrators when `apply_to_tenant_admins` is on, plus global administrators when `apply_to_global_admins` is on. TOTP or a security key counts. An OIDC login counts as having 2FA when the ID token `amr` contains one of `mfa`, `otp`, `hwk`, `swk`, `fpt` (or `oidc_accepted_amr`) or `acr` is in `oidc_accepted_acr`. LDAP users need a local factor. An IdP factor does not confirm a dangerous action; that still needs a DataSafe TOTP code or security key.

Grace (`grace_period_days`, 0–90, default 7) starts at policy enable or when the user later becomes a bucket administrator, whichever is later. During grace the console shows a banner and the actions below are allowed for a user who has not enrolled. After grace, those actions return `403 {"error":"mfa_required","message":"требуется 2FA"}`. The user can still read and write objects.

When the policy is off, confirmation matches 1.5.0. When it is on, every dangerous action by a covered user who already has a local factor needs a fresh code, including during grace. `POST /api/v1/auth/step-up` with `{action,target,totp_code}` returns a one-time token (`STORAGE_MFA_ACTION_TOKEN_TTL`, 30–300 seconds, default 60) bound to the user, session, action, and target. Send it as `X-DataSafe-MFA-Token`. Reuse, expiry, or a different user, action, or target is `401 {"error":"mfa_token_invalid"}`. A missing token is `401 {"error":"step_up_required","action":...}`. The same TOTP time step is accepted once for login or confirmation. Five failed confirmations in five minutes lock the user for five minutes.

S3 does not accept `X-DataSafe-MFA-Token`. An access key or STS session for a covered user can be created only from a confirmed session and is stored with `created_with_mfa=true`. `PutBucketEncryption`, `DeleteBucketEncryption`, `PutBucketPolicy`, `DeleteBucketPolicy`, `PutObjectLockConfiguration`, `PutObjectRetention` (COMPLIANCE or a shorter retain-until), `PutBucketVersioning` suspend, and `DeleteBucket` with a key that lacks the flag return `403 AccessDenied` «ключ создан без 2FA; создайте новый ключ после входа с 2FA». `PutObject` still succeeds. The bootstrap root key has no owner and is not subject to this flag.

`MfaRequiredDenied` is emitted at most once a minute per user and action. `MfaPolicyChanged`, `MfaStepUpRequired`, `MfaStepUpVerified`, and `MfaReset` go to the outbox and Activity.

<!-- dangerous-actions:begin -->
| action | meaning |
| --- | --- |
| crypto_shred | destroy a KMS key (crypto-shred) |
| key_rotate | rotate a KMS key |
| trash_purge | purge trash |
| sharing_policy_change | any sharing-policy change, including turning sharing off |
| grants_change | change bucket grants |
| encryption_disable | turn bucket encryption off |
| object_lock_config | Object Lock configuration |
| compliance_retention | COMPLIANCE retention, including extend or shorten |
| bucket_delete | delete a bucket |
| mfa_reset | reset a user's MFA |
| revoke_all_shares | revoke every share link on a bucket |
| access_key_create | create an access key or STS session for a covered user |
<!-- dangerous-actions:end -->

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
