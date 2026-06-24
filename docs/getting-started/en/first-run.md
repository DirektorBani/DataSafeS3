English | **[Русский](../ru/first-run.md)**

# First run

## Prerequisites

- Docker and Docker Compose
- Ports 8080 (console), 9000 (S3), 3000 (Grafana) available

## Steps

```cmd
copy .env.example .env
docker compose up -d --build
```

### PostgreSQL metadata (recommended for production-like setup)

```env
STORAGE_METADATA_BACKEND=postgres
STORAGE_POSTGRES_PUBLISH_PORT=5433
```

```cmd
docker compose --profile postgres up -d --build
```

`storage-server` retries Postgres connectivity on startup when using the `postgres` profile — useful on slower hosts or right after `postgres` container boot.

## Verify

```cmd
curl http://localhost:9000/api/v1/health
```

Open **http://localhost:8080** — you should see the login page.

## Fresh install reset

```powershell
.\scripts\reset-fresh-install.ps1 -Postgres -ProjectName cursor_p
```

After reset, `GET /api/v1/setup/status` returns `needs_setup: true`.

## Next

- [Setup wizard](setup-wizard.md)
- [Onboarding](onboarding.md)
