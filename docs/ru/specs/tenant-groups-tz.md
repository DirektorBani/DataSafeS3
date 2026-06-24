**[English](../../en/specs/tenant-groups-tz.md)** | Русский

# ТЗ: Группы внутри tenant (Tenant Groups)

**Версия:** 1.0  
**Дата:** 2026-06-17  
**Статус:** Реализовано

---

## 1. Цели

1. Дать **tenant admin** механизм разграничения доступа к бакетам tenant через **именованные группы** (например «Finance», «Dev»).
2. При добавлении пользователя в tenant назначать одну или несколько групп — пользователь получает доступ только к бакетам этих групп (плюс явные grants и собственные бакеты).
3. Сохранить совместимость с существующими **`tenant_members`** и **`bucket_access_grants`**.
4. Глобальный **administrator** сохраняет полный доступ; **tenant_admin** — полный доступ к бакетам своего tenant.

## 2. Терминология

| Термин | Описание |
|--------|----------|
| **Tenant** | Арендатор (организация). Существующая сущность `tenants`. |
| **Tenant Group** | Именованная группа внутри tenant. Набор бакетов + уровень доступа. |
| **Group ↔ Buckets** | Tenant admin привязывает бакеты tenant к группе (many-to-many). |
| **User ↔ Group** | Tenant admin назначает пользователя в одну или несколько групп при создании/редактировании участника. |
| **Access level группы** | `read` — только чтение; `read_write` — чтение и запись объектов. |
| **Grant** | Явное разрешение `bucket_access_grants` на конкретный бакет для конкретного пользователя. |

## 3. Связь с существующими механизмами

### 3.1. `tenant_members`

- Без изменений семантики ролей: `tenant_admin`, `member`, `viewer`.
- **tenant_admin** всегда имеет полный доступ ко всем бакетам tenant (включая групповые ограничения).
- Роль `member` / `viewer` определяет **дефолтный** доступ, когда tenant **не использует группы** и на бакете **нет grants**.

### 3.2. `bucket_access_grants`

- Grants остаются; работают **параллельно** с группами (объединение прав).
- Если у пользователя есть grant на бакет — доступ по grant, даже без группы.
- Если на бакете есть grants, но у пользователя нет grant и нет группы с этим бакетом — доступ запрещён (кроме tenant_admin / владельца).

### 3.3. Правило доступа (union)

Доступ к бакету tenant для пользователя (не administrator/operator):

```
доступ = владелец
      OR tenant_admin этого tenant
      OR явный grant (can_read / can_write)
      OR бакет в одной из групп пользователя (по access_level группы)
      OR (tenant без групп AND бакет без grants AND членство в tenant по роли member/viewer)
```

**Запись:** дополнительно требуется `can_write` в grant, `read_write` в группе, роль `member`/`tenant_admin` при дефолтном доступе, или владелец.

### 3.4. Режим «группы — основной механизм»

Если в tenant создана **хотя бы одна группа**:

- Обычные участники (`member` / `viewer`) видят бакеты tenant **только** через свои группы + grants + собственные бакеты.
- Бакеты tenant, **не привязанные ни к одной группе**, доступны только **tenant_admin** (и владельцу бакета, grants).
- Участник **без назначенных групп** не видит tenant-бакеты (кроме grants / своих).

Если групп в tenant **нет** — поведение как до внедрения групп (все бакеты tenant по роли member/viewer).

## 4. Модель данных

### 4.1. `tenant_groups`

| Поле | Тип | Описание |
|------|-----|----------|
| `id` | TEXT PK | UUID |
| `tenant_id` | TEXT FK → tenants | |
| `name` | TEXT | Уникально в пределах tenant |
| `description` | TEXT | Опционально |
| `access_level` | TEXT | `read` или `read_write` |
| `created_at` | TIMESTAMPTZ | |

### 4.2. `tenant_group_buckets`

| Поле | Тип |
|------|-----|
| `group_id` | TEXT FK → tenant_groups |
| `bucket_key` | TEXT FK → buckets.storage_key |

PK: `(group_id, bucket_key)`

### 4.3. `tenant_group_members`

| Поле | Тип |
|------|-----|
| `group_id` | TEXT FK → tenant_groups |
| `user_id` | TEXT FK → users |

PK: `(group_id, user_id)`

### 4.4. BoltDB

Бакеты: `tenant_groups`, `tenant_group_buckets`, `tenant_group_members` (ключи `group_id:bucket_key`, `group_id:user_id`).

## 5. API

Все эндпоинты требуют **tenant_admin** указанного tenant или глобального **administrator**.

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/tenants/{id}/groups` | Список групп tenant |
| POST | `/api/v1/tenants/{id}/groups` | Создать группу `{name, description?, access_level?}` |
| GET | `/api/v1/tenants/{id}/groups/{group_id}` | Детали группы + bucket_keys + member_count |
| PUT | `/api/v1/tenants/{id}/groups/{group_id}` | Обновить name, description, access_level |
| DELETE | `/api/v1/tenants/{id}/groups/{group_id}` | Удалить группу |
| PUT | `/api/v1/tenants/{id}/groups/{group_id}/buckets` | Заменить список бакетов `{bucket_keys: []}` |
| PUT | `/api/v1/tenants/{id}/members/{user_id}/groups` | Назначить группы пользователю `{group_ids: []}` |
| POST | `/api/v1/tenants/{id}/users` | Расширение: опционально `group_ids: []` |
| POST | `/api/v1/tenants/{id}/members` | Расширение: опционально `group_ids: []` |
| GET | `/api/v1/tenants/{id}/members` | В ответе: `groups: [{id, name}]` у каждого участника |

## 6. RBAC-матрица

| Действие | administrator | tenant_admin (свой tenant) | member | viewer |
|----------|---------------|------------------------------|--------|--------|
| CRUD групп | ✓ | ✓ | ✗ | ✗ |
| Назначение бакетов группе | ✓ | ✓ | ✗ | ✗ |
| Назначение групп участнику | ✓ | ✓ | ✗ | ✗ |
| Доступ к бакету через группу | ✓ | ✓ (все бакеты tenant) | по access_level | read только |
| Управление grants | ✓ | ✓ | ✗ | ✗ |
| Группы в другом tenant | ✓ | ✗ | ✗ | ✗ |

## 7. UI

### 7.1. Tenants → Groups

- Вкладка/секция **Groups** при выборе tenant (для tenant admin).
- Список групп: имя, access level, число бакетов/участников.
- Создание/редактирование: имя, описание, read / read_write.
- Multi-select бакетов tenant для группы.

### 7.2. Members

- При **Create user** и **Add member** — multi-select групп.
- В списке участников — badges с именами групп.
- Редактирование групп участника (PUT members/.../groups).

## 8. Миграция

- **PostgreSQL:** `005_tenant_groups.up.sql` — три новые таблицы.
- **BoltDB:** создание бакетов при `initEnterpriseBuckets`.
- Существующие tenants без групп — без изменений поведения.
- Миграция Bolt→Postgres: копирование групп, привязок бакетов и участников.

## 9. Критерии приёмки

1. Tenant admin создаёт группу, назначает бакеты и добавляет пользователя в группу — пользователь видит только эти бакеты.
2. Пользователь в группе `read` не может загружать объекты; в `read_write` — может.
3. tenant_admin видит и управляет всеми бакетами tenant независимо от групп.
4. Grant + группа работают как union (достаточно одного источника).
5. Cross-tenant: группы и назначения из tenant A не видны/не применимы в tenant B.
6. Участник без групп в tenant с группами не видит чужие tenant-бакеты.
7. `go test ./...`, `npm run build`, бинарник пересобирается.

## 10. SSO / LDAP → tenant groups

### 10.1. Принцип (MVP)

При входе через LDAP или OIDC DataSafeS3 читает внешние группы пользователя и **сопоставляет их по имени** с `tenant_groups` в каждом tenant, где пользователь состоит в `tenant_members`. Совпадение без учёта регистра. Ручное назначение групп в консоли сохраняется (union).

### 10.2. Настройки

| Источник | Поле в Settings | По умолчанию | Описание |
|----------|-----------------|--------------|----------|
| LDAP | `group_attr` | — | Атрибут групп на записи пользователя (`memberOf` в AD). Альтернатива: `group_dn` + поиск по `member` |
| OIDC | `groups_claim` | `groups` | Имя claim в JWT / userinfo (Keycloak Group Membership mapper) |

Глобальный `group_role_map` (LDAP) по-прежнему задаёт **глобальную** роль (`user` / `operator`), не tenant-группы.

### 10.3. Требования к IdP

1. Пользователь должен быть в `tenant_members` (добавлен tenant_admin или administrator).
2. В tenant заранее созданы группы с **теми же именами**, что во внешнем IdP (например Keycloak group `audit-finance` → tenant group `audit-finance`).
3. Keycloak: mapper **Group Membership** в access token; claim name = значение `groups_claim`.
4. LDAP/AD: атрибут `memberOf` или поиск по `group_dn`.

### 10.4. Ограничения MVP

- Нет отдельной таблицы «внешняя группа → tenant group ID»; только match по имени.
- Нет auto-create tenant group из IdP (создаёт tenant_admin).
- Синхронизация при login, не по расписанию.

Документация по Keycloak/LDAP: [ldap-keycloak-standalone.md](../integrations/ldap-keycloak-standalone.md).

## 11. Ссылки

- [tenant-bucket-isolation-tz.md](./tenant-bucket-isolation-tz.md) — scope, grants, tenant_admin
- [user-guide/README.md](../user-guide/README.md) — раздел Tenants / Groups
