English | **[Русский](../ru/tenants.md)**

# Tenant management

![Tenants](../../images/screenshots/tenants.png)

Multi-tenancy isolates organizations with separate bucket namespaces and member roles.

## Roles

| Role | Scope |
|------|-------|
| `tenant_admin` | Members, groups, bucket grants in tenant |
| `member` | Read/write per grants |
| `viewer` | Read-only |

## Console

**Administration → Tenants** — create tenants, add members, manage groups.

## API

```http
GET  /api/v1/tenants
POST /api/v1/tenants
POST /api/v1/tenants/{id}/members
POST /api/v1/tenants/{id}/users
```

## Specification

[Tenant bucket isolation TZ](../../en/specs/tenant-bucket-isolation-tz.md)
