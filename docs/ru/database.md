**[English](../en/database.md)** | Русский

# База данных

**Датасейф S3** хранит **метаданные** во встроенной СУБД. **Тела объектов** лежат в файловой системе (`STORAGE_DATA_DIR/objects/`) и в SQL/Bolt не попадают.

## Бэкенды метаданных

| Бэкенд | Переменная | По умолчанию | Миграции |
|--------|------------|--------------|----------|
| **BoltDB** | `STORAGE_METADATA_BACKEND=bolt` | Да (dev, single-node) | Схема через buckets в `internal/metadata/store.go` |
| **PostgreSQL 16** | `STORAGE_METADATA_BACKEND=postgres` | Production / Helm | `internal/metadata/postgres/migrations/*.sql` |

Схема PostgreSQL применяется при старте `storage-server` (`internal/metadata/postgres/store.go`). Версии — таблица `schema_migrations`.

Перенос Bolt → Postgres: `storage-server migrate-boltdb` (`cmd/storage-server/migrate.go`).

## ER-диаграмма (PostgreSQL)

Диаграмма ниже отображается на GitHub (Mermaid).

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

## Таблицы (PostgreSQL)

### Идентичность и тенанты

| Таблица | PK | Описание |
|---------|-----|----------|
| `tenants` | `id` | Изоляция тенантов; запись по умолчанию `default` |
| `users` | `id` | Пользователи консоли; глобальная роль |
| `tenant_members` | `(tenant_id, user_id)` | Роль внутри тенанта |
| `teams` | `id` | Команды для видимости бакетов |
| `user_teams` | `(user_id, team_id)` | Связь пользователь ↔ команда |
| `access_keys` | `access_key` | Ключи S3 (SigV4) |
| `api_tokens` | `id` | Токены консоли `ds_*` (хэш в `token_hash`) |

### Бакеты и объекты

| Таблица | PK | Примечание |
|---------|-----|------------|
| `buckets` | `storage_key` | Логическое `name` в scope тенанта или владельца (миграция 004) |
| `objects` | `(bucket, key, version_id)` | FK → `buckets(storage_key)` CASCADE |
| `multipart_uploads` | `upload_id` | Незавершённые multipart-загрузки |
| `shared_links` | `id` | Публичные ссылки; FK → `buckets` |
| `trash` | `id` | Корзина (soft delete) |
| `favorites` | `(user_id, id)` | Избранное в UI |

### Расширенный контроль доступа

| Таблица | PK | Описание |
|---------|-----|----------|
| `bucket_access_grants` | `(bucket_key, user_id)` | Права read/write на бакет для пользователя |
| `tenant_groups` | `id` | Именованные группы бакетов в тенанте |
| `tenant_group_buckets` | `(group_id, bucket_key)` | Бакеты в группе |
| `tenant_group_members` | `(group_id, user_id)` | Участники группы |

### Gateway и репликация

| Таблица | Назначение |
|---------|------------|
| `gateway_connections` | Удалённые S3-эндпоинты |
| `replication_rules` | Правила репликации |
| `replication_tasks` | Очередь задач репликации |
| `replication_errors` | Ошибки репликации |
| `sync_jobs` | Статус полной синхронизации |
| `federation_clusters` | Узлы федерации |
| `gateway_stats` | Сводные метрики (`id=global`) |

### Платформа

| Таблица | Назначение |
|---------|------------|
| `webhooks` / `webhook_deliveries` | Вебхуки и попытки доставки |
| `audit_logs` | Журнал действий админки |
| `system_config` | LDAP, OIDC, MFA, логирование (JSON) |
| `usage_counters` / `usage_snapshots` | Учёт использования |
| `schema_migrations` | Версии SQL-миграций |

## Ключевые связи

1. **Идентификатор бакета** — с миграции 004 PK — `storage_key`; `name` уникален в рамках тенанта или владельца.
2. **Изоляция тенантов** — индексы `idx_buckets_scope_tenant_name`, `idx_buckets_scope_owner_name`.
3. **Слои доступа** — глобальная роль → `tenant_members` → `bucket_access_grants` → `tenant_groups` (`internal/auth/rbac.go`).

## Индексы (основные)

| Индекс | Таблица | Назначение |
|--------|---------|------------|
| `idx_users_username_trgm` | `users` | Поиск по username (GIN) |
| `idx_buckets_name_trgm` | `buckets` | Поиск по имени бакета |
| `idx_objects_key_trgm` | `objects` | Поиск по ключу объекта |
| `idx_objects_latest` | `objects` | Частичный индекс `is_latest` |
| `idx_objects_prefix` | `objects` | Листинг по префиксу |

Расширение: `pg_trgm` (миграция 001).

## Миграции

| Версия | Файл | Изменение |
|--------|------|-----------|
| 1 | `001_init.up.sql` | Базовая схема |
| 2 | `002_teams_buckets.up.sql` | Команды, `owner_id`/`team_id` у бакетов |
| 3 | `003_shared_links_tenant_members.up.sql` | Ссылки и членство в тенанте |
| 4 | `004_bucket_scope_grants.up.sql` | `storage_key`, гранты доступа |
| 5 | `005_tenant_groups.up.sql` | Группы тенанта |
| 6 | `006_tenant_group_external_name.up.sql` | `external_name` для IdP |

Откат в репозитории: только `006_*.down.sql`.

## BoltDB (бэкенд по умолчанию)

Файл `{STORAGE_DATA_DIR}/metadata.db` (права `0600`). Те же сущности, что и в Postgres — см. `internal/metadata/store.go`.

Общий интерфейс: `internal/metadata/store_iface.go`.

## См. также

- [Архитектура](context/architecture.md)
- [Руководство — tenant groups](user-guide/README.md#tenant-groups-группы-арендатора) — модель доступа
