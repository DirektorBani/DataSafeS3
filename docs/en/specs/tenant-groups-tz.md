English | **[Русский](../../ru/specs/tenant-groups-tz.md)**

# Spec: Tenant Groups

**Version:** 1.0  
**Date:** 2026-06-17  
**Status:** Implemented

---

## 1. Goals

1. Give **tenant admin** a mechanism to scope bucket access within a tenant via **named groups** (e.g. "Finance", "Dev").
2. When adding a user to a tenant, assign one or more groups — the user gets access only to buckets in those groups (plus explicit grants and own buckets).
3. Preserve compatibility with existing **`tenant_members`** and **`bucket_access_grants`**.
4. Global **administrator** retains full access; **tenant_admin** — full access to buckets in their tenant.

## 2. Terminology

| Term | Description |
|--------|----------|
| **Tenant** | Tenant (organization). Existing `tenants` entity. |
| **Tenant Group** | Named group within a tenant. Set of buckets + access level. |
| **Group ↔ Buckets** | Tenant admin binds tenant buckets to a group (many-to-many). |
| **User ↔ Group** | Tenant admin assigns user to one or more groups when creating/editing a member. |
| **Group access level** | `read` — read only; `read_write` — read and write objects. |
| **Grant** | Explicit `bucket_access_grants` permission on a specific bucket for a specific user. |

## 3. Relationship to Existing Mechanisms

### 3.1. `tenant_members`

- Role semantics unchanged: `tenant_admin`, `member`, `viewer`.
- **tenant_admin** always has full access to all tenant buckets (including group restrictions).
- Role `member` / `viewer` defines **default** access when tenant **does not use groups** and bucket **has no grants**.

### 3.2. `bucket_access_grants`

- Grants remain; work **in parallel** with groups (union of permissions).
- If user has grant on bucket — access per grant, even without group.
- If bucket has grants but user has no grant and no group with that bucket — access denied (except tenant_admin / owner).

### 3.3. Access rule (union)

Tenant bucket access for user (not administrator/operator):

```
access = owner
      OR tenant_admin of this tenant
      OR explicit grant (can_read / can_write)
      OR bucket in one of user's groups (per group access_level)
      OR (tenant without groups AND bucket without grants AND tenant membership by member/viewer role)
```

**Write:** additionally requires `can_write` in grant, `read_write` in group, `member`/`tenant_admin` role for default access, or ownership.

### 3.4. "Groups as primary mechanism" mode

If tenant has **at least one group**:

- Regular members (`member` / `viewer`) see tenant buckets **only** via their groups + grants + own buckets.
- Tenant buckets **not bound to any group** are accessible only to **tenant_admin** (and bucket owner, grants).
- Member **without assigned groups** does not see tenant buckets (except grants / own).

If tenant has **no groups** — behavior as before groups (all tenant buckets per member/viewer role).

## 4. Data Model

### 4.1. `tenant_groups`

| Field | Type | Description |
|------|-----|----------|
| `id` | TEXT PK | UUID |
| `tenant_id` | TEXT FK → tenants | |
| `name` | TEXT | Unique within tenant |
| `description` | TEXT | Optional |
| `access_level` | TEXT | `read` or `read_write` |
| `created_at` | TIMESTAMPTZ | |

### 4.2. `tenant_group_buckets`

| Field | Type |
|------|-----|
| `group_id` | TEXT FK → tenant_groups |
| `bucket_key` | TEXT FK → buckets.storage_key |

PK: `(group_id, bucket_key)`

### 4.3. `tenant_group_members`

| Field | Type |
|------|-----|
| `group_id` | TEXT FK → tenant_groups |
| `user_id` | TEXT FK → users |

PK: `(group_id, user_id)`

### 4.4. BoltDB

Buckets: `tenant_groups`, `tenant_group_buckets`, `tenant_group_members` (keys `group_id:bucket_key`, `group_id:user_id`).

## 5. API

All endpoints require **tenant_admin** of the specified tenant or global **administrator**.

| Method | Path | Description |
|-------|------|----------|
| GET | `/api/v1/tenants/{id}/groups` | List tenant groups |
| POST | `/api/v1/tenants/{id}/groups` | Create group `{name, description?, access_level?}` |
| GET | `/api/v1/tenants/{id}/groups/{group_id}` | Group details + bucket_keys + member_count |
| PUT | `/api/v1/tenants/{id}/groups/{group_id}` | Update name, description, access_level |
| DELETE | `/api/v1/tenants/{id}/groups/{group_id}` | Delete group |
| PUT | `/api/v1/tenants/{id}/groups/{group_id}/buckets` | Replace bucket list `{bucket_keys: []}` |
| PUT | `/api/v1/tenants/{id}/members/{user_id}/groups` | Assign groups to user `{group_ids: []}` |
| POST | `/api/v1/tenants/{id}/users` | Extension: optional `group_ids: []` |
| POST | `/api/v1/tenants/{id}/members` | Extension: optional `group_ids: []` |
| GET | `/api/v1/tenants/{id}/members` | Response includes `groups: [{id, name}]` per member |

## 6. RBAC Matrix

| Action | administrator | tenant_admin (own tenant) | member | viewer |
|----------|---------------|------------------------------|--------|--------|
| Group CRUD | ✓ | ✓ | ✗ | ✗ |
| Assign buckets to group | ✓ | ✓ | ✗ | ✗ |
| Assign groups to member | ✓ | ✓ | ✗ | ✗ |
| Bucket access via group | ✓ | ✓ (all tenant buckets) | per access_level | read only |
| Manage grants | ✓ | ✓ | ✗ | ✗ |
| Groups in another tenant | ✓ | ✗ | ✗ | ✗ |

## 7. UI

### 7.1. Tenants → Groups

- **Groups** tab/section when tenant is selected (for tenant admin).
- Group list: name, access level, bucket/member counts.
- Create/edit: name, description, read / read_write.
- Multi-select tenant buckets for group.

### 7.2. Members

- On **Create user** and **Add member** — multi-select groups.
- Member list — badges with group names.
- Edit member groups (PUT members/.../groups).

## 8. Migration

- **PostgreSQL:** `005_tenant_groups.up.sql` — three new tables.
- **BoltDB:** bucket creation in `initEnterpriseBuckets`.
- Existing tenants without groups — no behavior change.
- Bolt→Postgres migration: copy groups, bucket bindings, and members.

## 9. Acceptance Criteria

1. Tenant admin creates group, assigns buckets, adds user to group — user sees only those buckets.
2. User in `read` group cannot upload objects; in `read_write` — can.
3. tenant_admin sees and manages all tenant buckets regardless of groups.
4. Grant + group work as union (one source sufficient).
5. Cross-tenant: groups and assignments from tenant A not visible/applicable in tenant B.
6. Member without groups in tenant with groups does not see other tenant buckets.
7. `go test ./...`, `npm run build`, binary rebuilds.

## 10. SSO / LDAP → Tenant Groups

### 10.1. Principle (MVP)

On LDAP or OIDC login DataSafeS3 reads user's external groups and **matches them by name** with `tenant_groups` in each tenant where user is in `tenant_members`. Case-insensitive match. Manual group assignment in console is preserved (union).

### 10.2. Settings

| Source | Settings field | Default | Description |
|----------|-----------------|--------------|----------|
| LDAP | `group_attr` | — | Group attribute on user record (`memberOf` in AD). Alternative: `group_dn` + search by `member` |
| OIDC | `groups_claim` | `groups` | Claim name in JWT / userinfo (Keycloak Group Membership mapper) |

Global `group_role_map` (LDAP) still sets **global** role (`user` / `operator`), not tenant groups.

### 10.3. IdP Requirements

1. User must be in `tenant_members` (added by tenant_admin or administrator).
2. Tenant must have groups created **with same names** as in external IdP (e.g. Keycloak group `audit-finance` → tenant group `audit-finance`).
3. Keycloak: **Group Membership** mapper in access token; claim name = `groups_claim` value.
4. LDAP/AD: `memberOf` attribute or search via `group_dn`.

### 10.4. MVP Limitations

- No separate "external group → tenant group ID" table; name match only.
- No auto-create tenant group from IdP (tenant_admin creates).
- Sync on login, not scheduled.

Keycloak/LDAP documentation: [ldap-keycloak-standalone.md](../integrations/ldap-keycloak-standalone.md).

## 11. References

- [tenant-bucket-isolation-tz.md](./tenant-bucket-isolation-tz.md) — scope, grants, tenant_admin
- [user-guide/README.md](../user-guide/README.md#tenant-groups) — Tenants / Groups section
