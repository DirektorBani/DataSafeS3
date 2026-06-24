English | **[Русский](../ru/users.md)**

# Users and RBAC

![Users](../../images/screenshots/users.png)

## Roles

| Role | Capabilities |
|------|--------------|
| `administrator` | Full access: users, settings, tenants, gateway |
| `operator` | All buckets/objects, no user administration |
| `user` | Own buckets and keys |

## Console

**Administration → Users** — create, edit, disable users; assign roles and quotas.

## API

```http
GET  /api/v1/users
POST /api/v1/users
PUT  /api/v1/users/{id}
```

## Tenant roles

Within tenants: `tenant_admin`, `member`, `viewer`. See [tenants.md](tenants.md).

## Full guide

[User guide — Administration](../../en/user-guide/05-administration.md)
