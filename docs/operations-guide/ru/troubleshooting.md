**[English](../en/troubleshooting.md)** | Русский

# Устранение неполадок

## Частые проблемы

| Симптом | Причина | Решение |
|---------|---------|---------|
| `setup_required` 403 | Мастер не завершён | Завершить `/setup` или `POST /setup/complete` |
| Docker build падает на Windows | WinHTTP proxy `127.0.0.1:10801` | `scripts\dev-docker-local-binary.cmd` |
| Console 404 при refresh | Том консоли пуст | `docker compose up console-assets`, затем `docker compose up -d caddy` (образ по умолчанию или `-f deploy/compose/docker-compose.local-console.yml` после `npm run build`) |
| S3 403 SignatureDoesNotMatch | Неверные ключи или время | `STORAGE_ACCESS_KEY`, синхронизация NTP |
| PostgreSQL connection refused | Профиль не запущен | `docker compose --profile postgres up -d` |

## Логи

```bash
docker compose logs -f storage-server
```

`STORAGE_LOG_LEVEL=debug` для подробного вывода.

## Проверки работоспособности

```bash
curl http://localhost:9000/api/v1/health
curl http://localhost:9000/metrics
```

## Ещё

[Локальная разработка](../../ru/context/local-dev.md) · [Устранение неполадок в руководстве пользователя](../../ru/user-guide/README.md)
