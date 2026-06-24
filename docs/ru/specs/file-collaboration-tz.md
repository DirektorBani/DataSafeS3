**[English](../../en/specs/file-collaboration-tz.md)** | Русский

# ТЗ: Файловая коллаборация — веб «Мои файлы» (Фаза 1)

**Версия:** 1.1  
**Дата:** 2026-06-23  
**Статус:** **Фазы 1–3 реализованы** · Фаза 4 mobile — [беклог](./file-collaboration-phase4-backlog.md) · см. [file-collaboration-status.md](./file-collaboration-status.md)  
**Родитель:** [roadmap-to-9.md](../../analysis/roadmap-to-9.md) (сегмент: рабочее место end-user)  
**Предусловие:** [tenant-bucket-isolation-tz.md](./tenant-bucket-isolation-tz.md) — **Реализовано**  
**Лицензия:** Apache-2.0 Community Edition — без платных gate  
**Аудитория:** Агент-разработчик / разработчик

---

## 0. Быстрый старт для агента

1. Прочитать ТЗ целиком; реализовать **только Фазу 1**, если пользователь явно не запросил Фазу 2+.
2. Искать существующий код перед новыми API:
   - `internal/api/bucket_access_handlers.go` — grants (сейчас только `tenant_admin`)
   - `internal/api/bucket_access.go` — `bucketListFilter`, `grantBucketKeysForUser`
   - `internal/metadata/teams.go` — `BucketListFilter`
   - `web/console/src/pages/bucket-detail.tsx` — вкладка Access
   - `web/console/src/pages/buckets.tsx` — список и создание бакетов
3. **Не ломать** семантику S3: логические имена, `storage_key`, существующие RBAC-тесты.
4. После реализации:
   - `go test ./...`
   - `scripts/feature-audit-test.ps1` (93/93 PASS)
   - Go-тесты на grants владельца и обогащение списка
   - Playwright или расширение audit script для сценария шаринга
   - Обновить **EN + RU** пользовательскую документацию по [product-documentation-tz.md](./product-documentation-tz.md) — ценность продукта, **без сравнений с конкурентами**
5. OpenAPI: расширить `docs/api/openapi.yaml` Tier A при добавлении стабильных JSON-маршрутов.

---

## 1. Цель

Дать каждому аутентифицированному пользователю **личное файловое пространство** в веб-консоли («**Мои файлы**») и позволить **владельцам** и **администраторам тенанта** делиться бакетами с выбранными пользователями — на базе существующего object storage и `bucket_access_grants`, без десктоп/мобильной синхронизации.

**Результаты:**

- Пользователь входит → видит личное хранилище (домашний бакет) и файлы, расшаренные коллегами.
- Владелец бакета выдаёт read/write доступ выбранным пользователям из консоли.
- `tenant_admin` продолжает управлять командными бакетами (существующий поток, единый UX).
- Позиционирование остаётся **управляемое self-hosted object storage** с элементами коллаборации — без заявления о полной desktop-синхронизации (Фаза 3+).

---

## 2. Проблема

| ID | Проблема | Доказательство |
|----|----------|----------------|
| FC-1 | Консоль говорит «S3-бакеты», а не «мои файлы» | `nav:buckets`, admin-centric sidebar |
| FC-2 | API `bucket_access_grants` только для `tenant_admin` | `handleListBucketAccess` |
| FC-3 | Нет личного пространства при первом входе | Пользователь создаёт бакет вручную |
| FC-4 | Общие бакеты не выделены в UI | `GET /buckets` без `ownership` |
| FC-5 | Владелец не может шарить **личный** бакет | Access tab только для `tenant_admin` |
| FC-6 | Нет аудита grant changes от владельца | Activity log частично |

---

## 3. Глоссарий

| Термин | Значение |
|--------|----------|
| **Домашний бакет** | Авто-созданный личный бакет в scope владельца |
| **Свой бакет** | `bucket.owner_id == текущий пользователь` |
| **Общий бакет** | Запись в `bucket_access_grants`, пользователь не владелец |
| **Бакет тенанта** | `tenant_id` задан и ≠ `default` |
| **Grant** | Строка `bucket_access_grants`: `can_read`, `can_write` |
| **Share** | Назначение grants пользователям (не публичные share links) |

---

## 4. Текущее состояние (не переimplementировать)

| Слой | Статус | Расположение |
|------|--------|--------------|
| Scope бакета по владельцу | Готово | `metadata.BucketScopeForUser` |
| Таблица `bucket_access_grants` | Готово | `bucket_grants.go` |
| Список бакетов с grants | Готово | `bucketListFilter` |
| S3 через grants | Готово | `s3/bucket_access.go` |
| Access tab tenant admin | Готово | `bucket-detail.tsx` |
| Создание бакета пользователем | Готово | `POST /api/v1/buckets/{name}` |

---

## 5. Scope

### Фаза 1 — In scope

| ID | Результат |
|----|-----------|
| P1-1 | Авто-создание **домашнего бакета** при первом входе |
| P1-2 | Обогащение `GET /api/v1/buckets`: `ownership`, `can_write`, `shared_by` |
| P1-3 | API grants на уровне **владельца** |
| P1-4 | Консоль: **Мои файлы** / **Общие со мной** |
| P1-5 | Диалог **Поделиться** на странице бакета |
| P1-6 | Выбор пользователей (см. FR-5) |
| P1-7 | События Activity log при изменении grants |
| P1-8 | i18n EN/RU/de/fr |
| P1-9 | Тесты + документация |

### Фаза 1 — Out of scope

- Grants на уровне папки (prefix)
- Desktop sync (Mac/Win/Linux)
- Мобильные приложения
- Push/email уведомления
- Редактирование документов в браузере
- Замена share links

### Фаза 2+ (отдельное ТЗ)

| Фаза | Фокус |
|------|-------|
| **2** | Prefix grants, недавние/избранное |
| **3** | Desktop (Tauri), sync MVP |
| **4** | Mobile viewer + upload |

---

## 6. Функциональные требования (Фаза 1)

### FR-1 Авто-создание домашнего бакета

Настройки (env):

| Параметр | Env | По умолчанию |
|----------|-----|--------------|
| Включить | `STORAGE_AUTO_HOME_BUCKET` | `true` |
| Имя | `STORAGE_HOME_BUCKET_NAME` | `files` |
| Видимость | — | `private` |

**Триггер:** успешный login или первый `GET /api/v1/me`, если у пользователя **0 своих бакетов** в `ScopeOwner`.

Создание через существующий путь create; идемпотентность при повторном вызове.

### FR-2 Обогащение списка бакетов

Добавить объект `access`:

```json
"access": {
  "ownership": "owned|shared|tenant",
  "can_read": true,
  "can_write": false,
  "shared_by": "alice"
}
```

Опционально: `?filter=owned|shared|all`.

### FR-3 API grants владельца

| Method | Path | Кто вызывает |
|--------|------|--------------|
| GET/PUT | `/api/v1/buckets/{bucket}/access` | владелец, `tenant_admin`, `administrator` |
| DELETE | `/api/v1/buckets/{bucket}/access/{user_id}` | те же |

Тело grants — как у tenant route. Рефакторинг: общий helper для tenant и owner routes.

**MVP правило grantee:** пользователь из общего тенанта с владельцем ИЛИ `administrator` назначает.

**Activity:** `ActionSettingsChanged`, resource `bucket_access`.

### FR-4 Навигация «Файлы»

- Переименовать **Бакеты** → **Файлы** для роли `user` (i18n `nav:files`).
- Вкладки: **Мои файлы** | **Общие со мной** | **Команда** (опционально).
- Пустое состояние с CTA.

### FR-5 Диалог «Поделиться»

Показывать вкладку Share если: admin, `tenant_admin`, или **владелец бакета**.

Новый endpoint для picker:

`GET /api/v1/shareable-users?bucket={name}&q={search}`

Сохранение: `PUT /api/v1/buckets/{bucket}/access`.

### FR-6 Упрощённый chrome для `user`

Скрыть admin-only разделы (проверить Gateway и т.д.).

### FR-7 Документация

Обновить use-case corporate-file-storage, user guide, what-is-datasafe (EN+RU). **Запрещены** сравнения с конкурентами.

---

## 7. Нефункциональные требования

| ID | Требование |
|----|------------|
| NFR-1 | Grant API p95 < 200 ms при ≤100 grants |
| NFR-2 | RBAC-тесты не регрессируют |
| NFR-3 | feature-audit 93/93 PASS |
| NFR-4 | EN/RU строки полные |
| NFR-5 | OpenAPI drift check |

---

## 8. Матрица RBAC

| Действие | administrator | user (владелец) | user (grantee) | tenant_admin |
|----------|:-------------:|:---------------:|:--------------:|:------------:|
| Список своих + общих | ✓ | ✓ | ✓ | ✓ |
| Share (PUT access) | ✓ | ✓ | — | ✓ |
| Запись в shared | per grant | per grant | per grant | per grant |

---

## 9. Критерии приёмки (DoD)

- [ ] **AC-1** Новый пользователь → бакет `files` (если auto включён).
- [ ] **AC-2** `GET /buckets` с `access.ownership`.
- [ ] **AC-3** Владелец шарит → grantee видит в «Общие со мной».
- [ ] **AC-4** Grantee с write загружает файл.
- [ ] **AC-5** Tenant admin flow без регрессии.
- [ ] **AC-6** Без grant — 403.
- [ ] **AC-7** Activity log.
- [ ] **AC-8** `go test ./...` PASS.
- [ ] **AC-9** feature-audit 93/93 PASS.
- [ ] **AC-10** Документация EN/RU.

---

## 10. Порядок реализации

1. Backend: home bucket + list enrichment.
2. Backend: owner grant API + рефакторинг tenant handlers.
3. `api.ts` типы.
4. UI: вкладки + Share.
5. Документация + i18n.
6. Тесты + audit.

**Оценка:** 7–10 недель (1 FTE full-stack) или 4–6 недель (backend + frontend параллельно).

---

## 11. Риски

| Риск | Митигация |
|------|-----------|
| Утечка списка пользователей | `shareable-users` только из общих тенантов |
| Режим `HasGrants` отрезает team | Документировать существующее поведение |

---

## Приложение — skill агента

[.cursor/skills/datasafe-file-collaboration/SKILL.md](../../../.cursor/skills/datasafe-file-collaboration/SKILL.md)

---

*ТЗ версия 1.0 · 2026-06-23*
