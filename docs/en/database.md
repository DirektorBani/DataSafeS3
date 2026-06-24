English | **[Русский](../ru/database.md)**

# Database

DataSafeS3 stores **metadata** in an embedded database. **Object bytes** live on the filesystem under `STORAGE_DATA_DIR/objects/` and are not in SQL/Bolt.

## Backends

| Backend | Env | Default | Migrations |
|---------|-----|---------|------------|
| **BoltDB** | `STORAGE_METADATA_BACKEND=bolt` | Yes (dev/single-node) | Schema via Go buckets in `internal/metadata/store.go` |
| **PostgreSQL 16** | `STORAGE_METADATA_BACKEND=postgres` | Production / Helm | `internal/metadata/postgres/migrations/*.sql` |

PostgreSQL schema is applied automatically on `storage-server` startup (`internal/metadata/postgres/store.go`). Version tracking: table `schema_migrations`.

Bolt → Postgres one-off migration: `storage-server migrate-boltdb` (`cmd/storage-server/migrate.go`).

## Entity-relationship diagram (PostgreSQL)

GitHub renders the diagram below natively (Mermaid).

```mermaid
erDiagram
    tenants ||--o{ users : "tenant_id"
    tenants ||--o{ tenant_members : ""
    users ||--o{ tenant_members : ""
    teams ||--o{ users : "team_id"
    teams ||--o{ user_teams : ""
    users ||--o{ user_teams : ""
    teams ||--o{ buckets : "team_id"

    buckets ||--o{ objects : "storage_key"
    buckets ||--o{ shared_links : ""
    buckets ||--o{ bucket_access_grants : ""
    users ||--o{ bucket_access_grants : ""

    tenants ||--o{ tenant_groups : ""
    tenant_groups ||--o{ tenant_group_buckets : ""
    buckets ||--o{ tenant_group_buckets : ""
    tenant_groups ||--o{ tenant_group_members : ""
    users ||--o{ tenant_group_members : ""

    webhooks ||--o{ webhook_deliveries : ""

    tenants {
        text id PK
        text name
        text status
    }

    users {
        text id PK
        text username UK
        text role
        text tenant_id FK
        text team_id FK
        boolean mfa_enabled
    }

    buckets {
        text storage_key PK
        text name
        text owner
        text tenant_id
        text team_id FK
        jsonb lifecycle_rules
    }

    objects {
        text bucket PK_FK
        text key PK
        text version_id PK
        bigint size
        boolean is_latest
    }

    tenant_groups {
        text id PK
        text tenant_id FK
        text name
        text access_level
        text external_name
    }

    bucket_access_grants {
        text bucket_key PK_FK
        text user_id PK_FK
        boolean can_read
        boolean can_write
    }
```

## Tables (PostgreSQL)

### Core identity & tenancy

| Table | Primary key | Description |
|-------|-------------|-------------|
| `tenants` | `id` | Multi-tenant isolation; default row `default` |
| `users` | `id` | Console users; global role (`administrator`, `operator`, `user`) |
| `tenant_members` | `(tenant_id, user_id)` | Per-tenant role (`tenant_admin`, `member`, `viewer`) |
| `teams` | `id` | Team grouping for bucket visibility |
| `user_teams` | `(user_id, team_id)` | User ↔ team membership |
| `access_keys` | `access_key` | S3 SigV4 credentials |
| `api_tokens` | `id` | Console API tokens (`ds_*`), stored as `token_hash` |

### Buckets & objects

| Table | Primary key | Notes |
|-------|-------------|-------|
| `buckets` | `storage_key` | Logical `name` scoped by `tenant_id` or `owner_id` (migration 004) |
| `objects` | `(bucket, key, version_id)` | FK → `buckets(storage_key)` ON DELETE CASCADE |
| `multipart_uploads` | `upload_id` | In-progress multipart uploads |
| `shared_links` | `id` | Public share tokens; FK → `buckets` |
| `trash` | `id` | Soft-deleted objects |
| `favorites` | `(user_id, id)` | User UI favorites |

### Access control extensions

| Table | Primary key | Description |
|-------|-------------|-------------|
| `bucket_access_grants` | `(bucket_key, user_id)` | Per-user read/write on a bucket |
| `tenant_groups` | `id` | Named bucket collections inside a tenant |
| `tenant_group_buckets` | `(group_id, bucket_key)` | Buckets in a group |
| `tenant_group_members` | `(group_id, user_id)` | Users in a group |

### Gateway & replication

| Table | Description |
|-------|-------------|
| `gateway_connections` | Remote S3 endpoints (keys stored in row) |
| `replication_rules` | Source bucket → destination connection |
| `replication_tasks` | Async replication queue items |
| `replication_errors` | Failed replication log |
| `sync_jobs` | Manual/full sync job status |
| `federation_clusters` | Registered peer clusters |
| `gateway_stats` | Singleton replication metrics (`id=global`) |

### Platform & observability

| Table | Description |
|-------|-------------|
| `webhooks` / `webhook_deliveries` | Event webhooks and delivery attempts |
| `audit_logs` | Admin activity log |
| `system_config` | LDAP, OIDC, MFA, logging JSON (`id=system`) |
| `usage_counters` | Global upload/download counters |
| `usage_snapshots` | Daily usage aggregates |
| `schema_migrations` | Applied SQL migration versions |

## Key relationships

1. **Bucket identity** — After migration 004, `buckets.storage_key` is the FK target; `name` is logical within tenant or owner scope.
2. **Tenant isolation** — `buckets.tenant_id` + partial unique indexes `idx_buckets_scope_tenant_name` / `idx_buckets_scope_owner_name`.
3. **Layered access** — Global user role → `tenant_members` → `bucket_access_grants` → `tenant_groups` (see `internal/auth/rbac.go`).

## Indexes (selected)

| Index | Table | Purpose |
|-------|-------|---------|
| `idx_users_username_trgm` | `users` | GIN trigram search on username |
| `idx_buckets_name_trgm` | `buckets` | GIN trigram search on bucket name |
| `idx_objects_key_trgm` | `objects` | GIN trigram search on object keys |
| `idx_objects_latest` | `objects` | Partial index `WHERE is_latest` |
| `idx_objects_prefix` | `objects` | Prefix listing (`text_pattern_ops`) |
| `idx_repl_tasks_status` | `replication_tasks` | Queue polling |
| `idx_api_tokens_hash` | `api_tokens` | Token lookup |

Extension: `pg_trgm` (migration 001).

## Migrations

| Version | File | Change |
|---------|------|--------|
| 1 | `001_init.up.sql` | Core schema (26 tables + `pg_trgm`) |
| 2 | `002_teams_buckets.up.sql` | `teams`, `user_teams`, bucket `owner_id`/`team_id` |
| 3 | `003_shared_links_tenant_members.up.sql` | `shared_links`, `tenant_members` |
| 4 | `004_bucket_scope_grants.up.sql` | `storage_key` PK, `bucket_access_grants` |
| 5 | `005_tenant_groups.up.sql` | Tenant groups and memberships |
| 6 | `006_tenant_group_external_name.up.sql` | `tenant_groups.external_name` (IdP mapping) |

Rollback: only `006_tenant_group_external_name.down.sql` is present in the repository.

## BoltDB (default backend)

Single file `{STORAGE_DATA_DIR}/metadata.db` (mode `0600`). Logical buckets include: `buckets`, `objects`, `users`, `tenants`, `access_keys`, `multipart`, `webhooks`, `gateway_connections`, `replication_rules`, `tenant_groups`, and others — see `internal/metadata/store.go` initialization.

Bolt and Postgres share the same Go domain structs (`internal/metadata/store_iface.go`).

## Related docs

- [Architecture](context/architecture.md) — component overview
- [User guide — tenant groups](user-guide/README.md#tenant-groups) — access model detail
