**[English](../en/onboarding.md)** | Русский

# Чеклист онбординга

Пошаговое руководство от нуля до рабочего DataSafeS3 с пользователями, тенантами и данными.

## Фаза 1 — Развёртывание

1. Клонируйте репозиторий, скопируйте `.env.example` → `.env`
2. Запустите стек: `docker compose --profile postgres up -d --build`
3. Проверьте работоспособность: `curl http://localhost:9000/api/v1/health`
4. Откройте **http://localhost:8080**

## Фаза 2 — Первый вход администратора

1. Войдите `admin` / `admin`
2. Перенаправление на **мастер настройки**
3. **Смените пароль** — обязательно, используйте надёжный пароль
4. На экране приветствия:
   - **Пропустить** внешний S3 (только локальное хранение), или
   - Настроить external S3 для репликации Gateway
5. **Завершить** — попадёте на главную (дашборд)

![Главная](../../images/screenshots/dashboard.png)

## Фаза 3 — Первая организация (tenant)

Тенанты изолируют бакеты и участников для подразделений или клиентов.

1. **Администрирование → Тенанты**
2. **Создать tenant** — например `Acme Corp`
3. Добавьте участников с ролями:
   - `tenant_admin` — управление участниками и grants
   - `member` — чтение/запись в выданных бакетах
   - `viewer` — только чтение

![Тенанты](../../images/screenshots/tenants.png)

См. [Руководство администратора — тенанты](../../administrator-guide/ru/tenants.md).

## Фаза 4 — Первые пользователи

1. **Администрирование → Пользователи**
2. Создайте пользователей с ролями:
   - `administrator` — полный доступ
   - `operator` — все бакеты, без управления пользователями
   - `user` — только свои бакеты
3. При необходимости назначьте в тенанты

![Пользователи](../../images/screenshots/users.png)

## Фаза 5 — Первый bucket и загрузка

1. **Бакеты → Создать** — имя `documents`, видимость `private`
2. Откройте bucket → **Загрузить** или перетащите файлы
3. Убедитесь, что объекты видны в браузере объектов

![Браузер объектов](../../images/screenshots/object-browser.png)

Альтернатива: [первый bucket через S3 CLI](first-bucket.md#через-s3-cli).

## Фаза 6 — Ключи доступа (опционально)

1. **Доступ → Ключи доступа** — S3-ключи для приложений
2. Или **токены API** (`ds_*`) для REST Admin API

## Фаза 7 — Усиление безопасности

| Задача | Где |
|--------|-----|
| Ротация секретов | `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY`, `STORAGE_ADMIN_PASSWORD`; `STORAGE_STRICT_SECRETS=true` |
| Проверка слабых значений | `GET /api/v1/settings/security-status` или баннер в консоли |
| Исходящие URL (логи, webhooks) | В prod — только публичный HTTPS; `STORAGE_DEV=true` для локального Loki |
| LDAP TLS | `ldaps://`; опционально `STORAGE_LDAP_REQUIRE_TLS=true` |
| OIDC ROPC | Отключить `STORAGE_OIDC_ROPC_ENABLED` в production |
| CORS | `STORAGE_CORS_ALLOWED_ORIGINS` (через запятую) |
| LDAP | Администрирование → Настройки → LDAP |
| OIDC / SSO | Администрирование → Настройки → OIDC |
| MFA | Профиль → Включить MFA |
| Смена S3 bootstrap key | Настройки или env `STORAGE_ACCESS_KEY` |

Перед production задайте `STORAGE_STRICT_SECRETS=true` — сервер не стартует, пока `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY` или `STORAGE_ADMIN_PASSWORD` совпадают с dev-дефолтами. В v1.0.2 также доступен `GET /api/v1/settings/security-status` (admin token) для pre-flight списка слабых переменных — то же предупреждение показывает баннер в консоли.

## Фаза 8 — Эксплуатация

| Задача | Где |
|--------|-----|
| Мониторинг | Grafana http://localhost:3000 (дашборд **DataSafeS3 Overview**) |
| Аудит | Администрирование → Активность |
| Backup | Копия `STORAGE_DATA_DIR` + дамп PostgreSQL — [руководство по эксплуатации](../../operations-guide/ru/backup-restore.md) |

## Быстрый API bootstrap

Для автоматизации (CI, скрипты):

```bash
# Вход → смена пароля → завершение настройки → создание bucket
# Полный пример: scripts/screenshots/capture.mjs
```

## Дальше

- [Руководство пользователя](../../ru/user-guide/README.md)
- [Руководство администратора](../../administrator-guide/ru/README.md)
- [Сценарии использования](../../use-cases/README.md)
