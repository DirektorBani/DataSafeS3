English | **[Русский](../../ru/specs/tenant-bucket-isolation-tz.md)**

# Spec: Tenant Bucket Isolation and Tenant Admin Access Control

**Version:** 1.0  
**Date:** 2026-06-17  
**Status:** Implemented

---

## 1. Goals

1. User can create a bucket with a name already taken in another tenant or by another owner.
2. Within one tenant (or owner's personal space) bucket name remains unique.
3. Tenant admin manages access to tenant buckets: who can read and write.
4. S3 API and console continue to show **logical bucket name**; physical storage is isolated via `storage_key`.

## 2. Current State

- `buckets` table: PK on `name` — global uniqueness.
- Fields: `name`, `owner_id`, `team_id`, `tenant_id`.
- Access: owner, team, tenant membership (roles **`tenant_admin`** / `member` / `viewer`). Canonical tenant administrator role name — **`tenant_admin`** (see [user guide](../user-guide/README.md#user-roles)).
- File storage: `data/buckets/<name>/`.

## 3. Data Model

### 3.1. Visibility scope

| Condition | Scope | Uniqueness |
|---------|-------|--------------|
| `tenant_id` set and ≠ `default` | tenant | `(tenant_id, name)` |
| Otherwise (personal / default tenant) | owner | `(owner_id, name)` |

### 3.2. Storage key

Internal identifier for metadata and filesystem:

- Tenant: `t:{tenant_id}:{name}`
- Owner: `o:{owner_id}:{name}`
- **Legacy migration:** `storage_key = name` (existing buckets, paths unchanged)

### 3.3. BucketRecord (extension)

```go
type BucketRecord struct {
    StorageKey string // PK in DB / key in Bolt
    Name       string // logical name for API/UI/S3
    // ... other fields unchanged
}
```

### 3.4. `bucket_access_grants` table

```sql
bucket_key  TEXT  -- FK → buckets.storage_key
user_id     TEXT  -- FK → users.id
can_read    BOOLEAN DEFAULT TRUE
can_write   BOOLEAN DEFAULT FALSE
PRIMARY KEY (bucket_key, user_id)
```

**Access rules when grants exist:**

- If bucket has at least one `bucket_access_grants` record, access is determined **only** by grants + owner + bucket's tenant `tenant_admin`.
- If grants empty — current rules apply (owner, team, tenant membership by role).

## 4. API

### 4.1. Existing endpoints (behavior)

- `POST /api/v1/buckets/{name}` — create in current user's scope; 409 only on conflict in same scope.
- `GET /api/v1/buckets` — list with logical names; with duplicates in different scopes user sees only their own.
- S3 `PUT /{bucket}` — resolve by principal (access key / user) in owner's scope.

### 4.2. New endpoints (tenant admin)

| Method | Path | Description |
|-------|------|----------|
| GET | `/api/v1/tenants/{tenant}/buckets/{bucket}/access` | List grants |
| PUT | `/api/v1/tenants/{tenant}/buckets/{bucket}/access` | Replace grants (`{grants: [{user_id, can_read, can_write}]}`) |
| DELETE | `/api/v1/tenants/{tenant}/buckets/{bucket}/access/{user_id}` | Remove grant |

Requirements: caller is **`tenant_admin`** of specified tenant (membership in `tenant_members`) or global **administrator**.

### 4.3. Tenant member management

| Method | Path | Who can |
|-------|------|-----------|
| GET | `/api/v1/tenants` | administrator (all) or tenant_admin (own tenants only) |
| POST/DELETE | `/api/v1/tenants` | administrator only |
| GET/POST/PUT/DELETE | `/api/v1/tenants/{id}/members` | tenant_admin of this tenant or administrator |
| POST | `/api/v1/tenants/{id}/users` | tenant_admin or administrator — create local user and add to tenant (tenant roles: `member` / `viewer`; `tenant_admin` — administrator only) |

`GET /api/v1/me` returns `tenant_memberships` and `is_tenant_admin`.

## 5. UI

1. **Bucket creation** — UX unchanged; 409 only on conflict in own scope.
2. **Tenants → Create user** (tenant admin): username / password / tenant role (`member` or `viewer`); user created with global role `user` and auto-added to tenant.
3. **Bucket detail → Access tab** (visible to tenant admin): tenant user table, read/write checkboxes, save via PUT access API.

## 6. Migration

### PostgreSQL (`004_bucket_scope_grants.up.sql`)

1. `ALTER TABLE buckets ADD COLUMN storage_key TEXT`
2. `UPDATE buckets SET storage_key = name`
3. PK moved to `storage_key`
4. Unique indexes on scope + name
5. FK `objects.bucket` → `buckets.storage_key`
6. Create `bucket_access_grants`

### BoltDB

- Bucket record key: `storage_key`
- Index `bucket_scope_index`: `scope\x00tenant|owner\x00name` → `storage_key`
- Bucket `bucket_access_grants`

**Backward compatibility:** existing buckets (`storage_key = name`) work without on-disk data migration.

## 7. Acceptance Criteria

- [ ] Two users in different tenants create bucket `data` — both succeed.
- [ ] Two users without tenant (default) create `data` — both succeed (different `owner_id`).
- [ ] Two users in same tenant create `data` — second gets 409.
- [ ] Tenant admin assigns read-only grant — user sees objects but cannot write.
- [ ] Tenant admin assigns grants — users without grant lose access (except owner/admin).
- [ ] S3 ListBuckets returns only buckets in principal's scope.
- [ ] Legacy buckets accessible by previous name.

## 8. Limitations and Breaking Changes

- **No breaking change** for existing unique names: `storage_key = name`.
- When two buckets share same logical name in different scopes, **global admin** resolving by name without context gets first match (as before with ambiguity — use tenant context).
- Replication/gateway `source_bucket` still uses logical name in rule owner's scope.
