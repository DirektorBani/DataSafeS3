**[English](../../en/specs/file-collaboration-status.md)** | Русский

# Файловая коллаборация — статус реализации

**Обновлено:** 2026-06-19  
**ТЗ:** [file-collaboration-tz.md](./file-collaboration-tz.md)  
**Документация:** `24c2cbe` · **Код:** фазы 1–3 в рабочей копии (коммит кода ожидается)

---

## Сводка

| Фаза | Scope | Статус |
|------|-------|--------|
| **1** | Веб «Мои файлы», home bucket, grants владельца, Share UI | **Реализовано** |
| **2** | Grants на папку, уведомления, квота home bucket | **Реализовано** |
| **3** | Desktop sync (`datasafe-sync` CLI + Tauri) | **Реализовано** |
| **4** | Mobile Flutter + mobile-web PWA | **Беклог** — [phase4-backlog](./file-collaboration-phase4-backlog.md) |

Честная позиция: **веб-хранилище + шаринг бакета/папки + desktop sync** (CLI и опционально Tauri). Mobile — только прототипы в репо, не поставка. Нет интеграции с файловым менеджером ОС и co-editing.

---

## Фаза 1 — Веб (готово)

**Backend:** `home_bucket.go`, обогащение `GET /buckets` (`access.ownership`, `can_read`, `can_write`, `shared_by`), фильтр `?filter=owned|shared|tenant|all`, `GET|PUT|DELETE /api/v1/buckets/{bucket}/access`, `GET /shareable-users`, тесты `home_bucket_test.go`.

**Консоль:** навигация «Файлы» (`sidebar.tsx`), вкладки Мои / Общие / Команда (`buckets.tsx`), Share tab + picker (`bucket-detail.tsx`), empty state для home bucket.

**Env:** `STORAGE_AUTO_HOME_BUCKET`, `STORAGE_HOME_BUCKET_NAME`, `STORAGE_HOME_BUCKET_MAX_SIZE_BYTES`.

**Документация:** [user guide — Файлы](../user-guide/README.md), [use case](../../use-cases/ru/corporate-file-storage.md).

---

## Фаза 2 — Папки и уведомления (готово)

Миграция `010`: `bucket_prefix_access_grants`, `user_notifications`, `recent_items`. BoltDB: `collaboration_phase2.go`, `prefix_grants.go`.

**API:** `/api/v1/notifications`, `POST /notifications/read-all`, prefix grants в PUT access, `access.shared_prefixes[]`, `/api/v1/recent`, prefix-aware list (`allowedListPrefix`, S3 `EffectiveListPrefixForAccessKey`).

**UI:** grants на prefix, колокольчик (`notification-bell.tsx`), префиксы во вкладке «Общие», recent на странице Files.

---

## Фаза 3 — Desktop (реализовано)

| Компонент | Путь |
|-----------|------|
| Движок sync | `internal/syncapp/` — pull/push, delete, конфликты, fsnotify |
| CLI | `cmd/datasafe-sync/` — login, sync, watch, buckets, conflicts, resolve, token set |
| Tauri UI | `clients/desktop/` — tray, picker, watch, конфликты |
| Сборка sidecar | `scripts/build-datasafe-sync.ps1`, `.sh` |

Политики конфликтов: `last_write_wins`, `local_wins`, `remote_wins`, `keep_both` (`.datasafe-conflicts/`). Code signing / auto-update — не в дереве.

Быстрый старт: [clients/README.md](../../../clients/README.md) · [desktop/README.md](../../../clients/desktop/README.md)

---

## Фаза 4 — Mobile (беклог)

Прототипы в `clients/mobile` и `clients/mobile-web` — не поставка. См. [беклог](./file-collaboration-phase4-backlog.md).

---

## Чего нет

| Возможность | Примечание |
|-------------|------------|
| Finder / Explorer sync | Нужен file provider или сторонние инструменты |
| Фоновый mobile sync | Фаза 4+ |
| Co-editing | Вне scope |
| Push / email уведомления | Только in-app |
| Аудит каждого просмотра файла | Логируются изменения grants |

---

## Проверка (2026-06-19)

| Проверка | Результат |
|----------|-----------|
| `go test ./...` | **PASS** |
| `home_bucket_test.go`, `syncapp` | PASS |
| feature-audit 93/93 | PASS (2026-06-22) |
| Console `npm run build` | PASS (2026-06-23) |

### Исправления (июнь 2026)

- List S3/API: prefix-only grantees не видят весь бакет
- `ownership=shared` для prefix-only grants
- Миграция `011_recent_items_user_id`
- DELETE access отзывает prefix grants
- Tauri: login перед sync
- `datasafe-sync`: сохранение folder/bucket/prefix в профиле

---

## Связанные документы

- [file-collaboration-tz.md](./file-collaboration-tz.md)
- [competitive-assessment-2026-v5.md](../../analysis/competitive-assessment-2026-v5.md) — **9.1/10**
- [tenant-bucket-isolation-tz.md](./tenant-bucket-isolation-tz.md)
