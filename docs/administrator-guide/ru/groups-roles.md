**[English](../en/groups-roles.md)** | Русский

# Группы и роли

**Группы** тенанта объединяют доступ к buckets для команд.

## Концепции

- **Системные роли** (`administrator`, `operator`, `user`) — глобальный RBAC
- **Группы тенанта** — именованные наборы grants внутри тенанта
- **Назначение участников** — пользователи получают членство в группах

## Консоль

**Администрирование → Tenants** → выберите тенант → **Groups**

## API

```http
GET  /api/v1/tenants/{id}/groups
POST /api/v1/tenants/{id}/groups
PUT  /api/v1/tenants/{id}/groups/{group_id}/buckets
PUT  /api/v1/tenants/{id}/members/{user_id}/groups
```

## Спецификация

[ТЗ tenant groups](../../ru/specs/tenant-groups-tz.md)
