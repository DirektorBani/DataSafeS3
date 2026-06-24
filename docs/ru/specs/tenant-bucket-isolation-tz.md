**[English](../../en/specs/tenant-bucket-isolation-tz.md)** | Русский

# ТЗ: Изоляция бакетов по тенанту и управление доступом tenant admin

**Версия:** 1.0  
**Дата:** 2026-06-17  
**Статус:** Реализовано

---

## 1. Цели

1. Пользователь может создать бакет с именем, которое уже занято в другом тенанте или у другого владельца.
2. Внутри одного тенанта (или личного пространства владельца) имя бакета остаётся уникальным.
3. Tenant admin управляет доступом к бакетам своего тенанта: кто может читать и писать.
4. S3 API и консоль продолжают показывать **логическое имя** бакета; физическое хранение изолировано через `storage_key`.

## 2. Текущее состояние

- Таблица `buckets`: PK по `name` — глобальная уникальность.
- Поля: `name`, `owner_id`, `team_id`, `tenant_id`.
- Доступ: владелец, команда, членство в тенанте (роли **`tenant_admin`** / `member` / `viewer`). Каноническое имя роли администратора арендатора — **`tenant_admin`** (см. [руководство пользователя](../user-guide/README.md#роли-пользователей)).
- Файловое хранилище: `data/buckets/<name>/`.

## 3. Модель данных

### 3.1. Область видимости (scope)

| Условие | Scope | Уникальность |
|---------|-------|--------------|
| `tenant_id` задан и ≠ `default` | tenant | `(tenant_id, name)` |
| Иначе (личный / default tenant) | owner | `(owner_id, name)` |

### 3.2. Storage key

Внутренний идентификатор для метаданных и файловой системы:

- Tenant: `t:{tenant_id}:{name}`
- Owner: `o:{owner_id}:{name}`
- **Миграция legacy:** `storage_key = name` (существующие бакеты без изменений путей)

### 3.3. BucketRecord (расширение)

```go
type BucketRecord struct {
    StorageKey string // PK в БД / ключ в Bolt
    Name       string // логическое имя для API/UI/S3
    // ... остальные поля без изменений
}
```

### 3.4. Таблица `bucket_access_grants`

```sql
bucket_key  TEXT  -- FK → buckets.storage_key
user_id     TEXT  -- FK → users.id
can_read    BOOLEAN DEFAULT TRUE
can_write   BOOLEAN DEFAULT FALSE
PRIMARY KEY (bucket_key, user_id)
```

**Правила доступа при наличии grants:**

- Если для бакета есть хотя бы одна запись в `bucket_access_grants`, доступ определяется **только** grants + владелец + `tenant_admin` тенанта бакета.
- Если grants пусты — действуют текущие правила (владелец, команда, членство в тенанте по роли).

## 4. API

### 4.1. Существующие эндпоинты (поведение)

- `POST /api/v1/buckets/{name}` — создание в scope текущего пользователя; 409 только при конфликте в том же scope.
- `GET /api/v1/buckets` — список с логическими именами; при дубликатах в разных scope пользователь видит только свои.
- S3 `PUT /{bucket}` — резолв по principal (access key / user) в scope владельца.

### 4.2. Новые эндпоинты (tenant admin)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/tenants/{tenant}/buckets/{bucket}/access` | Список grants |
| PUT | `/api/v1/tenants/{tenant}/buckets/{bucket}/access` | Заменить grants (`{grants: [{user_id, can_read, can_write}]}`) |
| DELETE | `/api/v1/tenants/{tenant}/buckets/{bucket}/access/{user_id}` | Удалить grant |

Требования: вызывающий — **`tenant_admin`** указанного тенанта (членство в `tenant_members`) или глобальный **administrator**.

### 4.3. Управление участниками tenant

| Метод | Путь | Кто может |
|-------|------|-----------|
| GET | `/api/v1/tenants` | administrator (все) или tenant_admin (только свои tenants) |
| POST/DELETE | `/api/v1/tenants` | только administrator |
| GET/POST/PUT/DELETE | `/api/v1/tenants/{id}/members` | tenant_admin этого tenant или administrator |
| POST | `/api/v1/tenants/{id}/users` | tenant_admin или administrator — создать локального пользователя и добавить в tenant (роли tenant: `member` / `viewer`; `tenant_admin` — только administrator) |

`GET /api/v1/me` возвращает `tenant_memberships` и `is_tenant_admin`.

## 5. UI

1. **Создание бакета** — без изменений UX; ошибка 409 только при конфликте в своём scope.
2. **Tenants → Create user** (tenant admin): форма username / password / tenant role (`member` или `viewer`); пользователь создаётся с глобальной ролью `user` и автоматически добавляется в tenant.
3. **Bucket detail → вкладка Access** (видна tenant admin): таблица пользователей тенанта, чекбоксы read/write, сохранение через PUT access API.

## 6. Миграция

### PostgreSQL (`004_bucket_scope_grants.up.sql`)

1. `ALTER TABLE buckets ADD COLUMN storage_key TEXT`
2. `UPDATE buckets SET storage_key = name`
3. PK переносится на `storage_key`
4. Уникальные индексы по scope + name
5. FK `objects.bucket` → `buckets.storage_key`
6. Создание `bucket_access_grants`

### BoltDB

- Ключ записи бакета: `storage_key`
- Индекс `bucket_scope_index`: `scope\x00tenant|owner\x00name` → `storage_key`
- Bucket `bucket_access_grants`

**Обратная совместимость:** существующие бакеты (`storage_key = name`) работают без переноса данных на диске.

## 7. Критерии приёмки

- [ ] Два пользователя в разных тенантах создают бакет `data` — оба успешны.
- [ ] Два пользователя без тенанта (default) создают `data` — оба успешны (разные `owner_id`).
- [ ] Два пользователя в одном тенанте создают `data` — второй получает 409.
- [ ] Tenant admin назначает read-only grant — пользователь видит объекты, но не может писать.
- [ ] Tenant admin назначает grants — пользователи без grant теряют доступ (кроме owner/admin).
- [ ] S3 ListBuckets возвращает только бакеты в scope principal.
- [ ] Legacy бакеты доступны по прежнему имени.

## 8. Ограничения и breaking changes

- **Нет breaking change** для существующих уникальных имён: `storage_key = name`.
- При появлении двух бакетов с одинаковым логическим именем в разных scope **глобальный admin** при обращении по имени без контекста получает первый найденный (как и раньше при неоднозначности — рекомендуется использовать tenant context).
- Репликация/gateway по `source_bucket` по-прежнему использует логическое имя в scope владельца правила.
