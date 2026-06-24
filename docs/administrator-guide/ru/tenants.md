**[English](../en/tenants.md)** | Русский

# Управление тенантами

![Tenants](../../images/screenshots/tenants.png)

Мульти-тенантность изолирует организации с отдельными пространствами имён bucket и ролями участников.

## Роли

| Роль | Область |
|------|---------|
| `tenant_admin` | Участники, группы, grants в тенанте |
| `member` | Чтение/запись по grants |
| `viewer` | Только чтение |

## Консоль

**Администрирование → Tenants** — создание, участники, группы.

## API

```http
GET  /api/v1/tenants
POST /api/v1/tenants
POST /api/v1/tenants/{id}/members
POST /api/v1/tenants/{id}/users
```

## Спецификация

[ТЗ изоляции bucket тенанта](../../ru/specs/tenant-bucket-isolation-tz.md)
