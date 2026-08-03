**[English](../en/first-run.md)** | Русский

# Первый запуск

## Требования

- Docker и Docker Compose
- Свободные порты 8080 (консоль), 9000 (S3), 3000 (Grafana)

## Шаги

**Рекомендуется:** [интерактивный установщик](installer.md) (`.\install.ps1` / `./install.sh`).

Вручную:

```cmd
copy .env.example .env
docker compose -p datasafe --profile postgres -f docker-compose.yml -f docker-compose.local-data.yml up -d
```

Опциональные overlays (HA, Vault, OAuth2, local binary, …): [`deploy/compose/README.md`](../../deploy/compose/README.md).

### PostgreSQL для метаданных (рекомендуется для production-like)

```env
STORAGE_METADATA_BACKEND=postgres
STORAGE_POSTGRES_PUBLISH_PORT=5433
```

```cmd
docker compose --profile postgres up -d --build
```

`storage-server` повторяет подключение к Postgres при старте в профиле `postgres` — удобно на медленных хостах или сразу после запуска контейнера `postgres`.

## Проверка

```cmd
curl http://localhost:9000/api/v1/health
```

Откройте **http://localhost:8080** — должна появиться страница входа.

## Сброс к чистой установке

```powershell
.\scripts\reset-fresh-install.ps1 -Postgres -ProjectName datasafe
```

После сброса `GET /api/v1/setup/status` возвращает `needs_setup: true`.

## Далее

- [Мастер настройки](setup-wizard.md)
- [Онбординг](onboarding.md)
