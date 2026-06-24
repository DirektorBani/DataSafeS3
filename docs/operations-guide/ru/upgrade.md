**[English](../en/upgrade.md)** | Русский

# Обновление

## Docker Compose

```bash
git pull
docker compose --profile postgres build storage-server
docker compose --profile postgres up -d
```

С local binary overlay (Windows dev):

```cmd
scripts\dev-docker-local-binary.cmd
```

## Миграции

Миграции PostgreSQL выполняются автоматически при старте `storage-server` (`internal/metadata/postgres/migrations/`).

## Откат

1. Остановить стек
2. Восстановить предыдущий binary/image и backup данных
3. Запустить стек

## Чеклист

- [ ] Backup метаданных и объектов
- [ ] Проверить changelog / миграции
- [ ] Тест на staging
- [ ] Пересобрать консоль при изменении UI: `scripts\build-console.cmd`
