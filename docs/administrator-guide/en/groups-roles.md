English | **[Русский](../ru/groups-roles.md)**

# Groups and roles

Tenant **groups** bundle bucket access for teams.

## Concepts

- **System roles** (`administrator`, `operator`, `user`) — global RBAC
- **Tenant groups** — named sets of bucket grants inside a tenant
- **Member assignment** — users get group membership for scoped access

## Console

**Administration → Tenants** → select tenant → **Groups**

## API

```http
GET  /api/v1/tenants/{id}/groups
POST /api/v1/tenants/{id}/groups
PUT  /api/v1/tenants/{id}/groups/{group_id}/buckets
PUT  /api/v1/tenants/{id}/members/{user_id}/groups
```

## Specification

[Tenant groups TZ](../../en/specs/tenant-groups-tz.md)
