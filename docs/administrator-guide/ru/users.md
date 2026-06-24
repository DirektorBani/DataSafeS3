**[English](../en/users.md)** | Русский

# Пользователи и RBAC

![Users](../../images/screenshots/users.png)

## Роли

| Роль | Возможности |
|------|-------------|
| `administrator` | Полный доступ: users, settings, tenants, gateway |
| `operator` | Все buckets/objects, без управления пользователями |
| `user` | Свои buckets и ключи |

## Консоль

**Администрирование → Users** — создание, редактирование, отключение; роли и квоты.

## API

```http
GET  /api/v1/users
POST /api/v1/users
PUT  /api/v1/users/{id}
```

## Роли в тенанте

`tenant_admin`, `member`, `viewer`. См. [tenants.md](tenants.md).

## Полное руководство

[Руководство пользователя — Администрирование](../../ru/user-guide/05-administraciya.md)
