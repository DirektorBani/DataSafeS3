**[English](../../en/specs/initial-setup-wizard-tz.md)** | Русский

# ТЗ: Мастер первичной настройки системы (Initial Setup Wizard)

**Версия:** 1.0  
**Дата:** 2026-06-20  
**Статус:** Реализовано  
**Связанные файлы:** `internal/api/setup_handlers.go`, `web/console/src/pages/setup.tsx`, `scripts/reset-fresh-install.ps1`

---

## 1. Цель

При первом запуске после чистой установки администратор проходит короткий мастер настройки: приветствие и подключение внешнего S3-хранилища. До завершения мастера остальные разделы консоли недоступны.

---

## 2. Сброс к чистой установке

Скрипт `scripts/reset-fresh-install.ps1` (или `.cmd`):

- останавливает Docker Compose;
- удаляет `metadata.db` и каталог `objects/` в `STORAGE_DATA_DIR`;
- для профиля `postgres` — `docker compose --profile postgres down -v`;
- перезапускает стек.

---

## 3. Поведение

| Шаг | Описание |
|-----|----------|
| 1 | Чистая БД: `initial_setup_completed=false`, `admin_first_login_completed=false`, `admin_password_changed=false` |
| 2 | Страница входа показывает подсказку `admin / admin`, пока `admin_first_login_completed=false` |
| 3 | Первый успешный вход администратора → `admin_first_login_completed=true`, редирект на `/setup` |
| 4 | **Обязательная смена пароля** (модальное окно): до смены пароля мастер недоступен; `POST /me/password` → `admin_password_changed=true` |
| 5 | Шаг «Добро пожаловать!» — можно начать настройку S3 или **пропустить** (завершить мастер без S3) |
| 6 | Форма S3: Endpoint, Access Key, Secret Key, Bucket, Region, Use SSL; «Проверить», «Сохранить» или **«Пропустить S3»** |
| 7 | Завершение (`POST /setup/s3/save` или `POST /setup/complete`) → `initial_setup_completed=true`, шаг «Готово», полный доступ |
| 8 | Прогресс-бар: 4 шага (Пароль → Приветствие → S3 → Готово) |
| 9 | При следующих входах мастер не показывается |

---

## 4. Модель данных (`SystemConfig`)

```json
{
  "initial_setup_completed": false,
  "admin_first_login_completed": false,
  "admin_password_changed": false,
  "external_s3": {
    "endpoint": "",
    "access_key_id": "",
    "secret_access_key": "",
    "bucket": "",
    "region": "",
    "use_ssl": true
  }
}
```

---

## 5. API

| Метод | Путь | Доступ | Описание |
|-------|------|--------|----------|
| GET | `/api/v1/setup/status` | публичный | `{ initial_setup_completed, admin_first_login_completed, admin_password_changed, needs_password_change, needs_setup }` |
| POST | `/api/v1/setup/s3/test` | admin | Проверка HeadBucket + PutObject в указанный bucket |
| POST | `/api/v1/setup/s3/save` | admin | Сохранение `external_s3`, `initial_setup_completed=true` |
| POST | `/api/v1/setup/complete` | admin | Завершить мастер без S3 (`initial_setup_completed=true`) |

При `!initial_setup_completed` администратору возвращается `403` с `{ "error": "setup_required" }` на всех API, кроме setup, login, `/me`, logout и health.

Первый вход администратора (`POST /admin/login`, MFA login или `GET /me`) выставляет `admin_first_login_completed=true`. Смена пароля (`POST /me/password`) выставляет `admin_password_changed=true`.

---

## 6. Решение по S3 (дизайн)

**Выбран вариант C+:** учётные данные внешнего S3 в `SystemConfig.external_s3`; проверка через AWS SDK v2 (HeadBucket + тестовый PutObject); при сохранении автоматически создаётся gateway-подключение `default` для репликации.

Первичное хранилище объектов остаётся **FSBackend** на локальном диске (`STORAGE_DATA_DIR/objects/`). Внешний S3 используется для резервного копирования и Gateway-репликации, не как основной backend.

---

## 7. Frontend

| Маршрут | Назначение |
|---------|------------|
| `/setup` | Мастер (welcome + S3), только для authed admin при `needs_setup` |
| `RequireSetupComplete` | Обёртка `AppLayout` — редирект на `/setup` |

---

## 8. Обратная совместимость

При обновлении существующей установки, если у любого пользователя есть `last_login`, оба флага setup автоматически устанавливаются в `true` при старте сервера.

---

## 9. Тесты

`internal/api/setup_handlers_test.go` — статус, guard, first login, save/test S3.

---

## 10. Документация

- [Руководство пользователя — первичная настройка](../user-guide/README.md#первичная-настройка)
- [English spec](../../en/specs/initial-setup-wizard-tz.md)
