# Production Compose

English: [deploy/compose/production/README.md](../../deploy/compose/production/README.md).

Один контейнер хранилища, метаданные Bolt, край Caddy и именованный том. В файле Compose `STORAGE_DEV` равен `false`. Caddy не проксирует `/metrics`. Порт S3 привязан только к `127.0.0.1`.

```bash
cp deploy/compose/production/.env.example deploy/compose/production/.env
# замените секреты
docker compose --env-file deploy/compose/production/.env \
  -f deploy/compose/production/docker-compose.yml up -d
```

Откройте `http://127.0.0.1:8080`, смените пароль администратора и завершите мастер. S3 на хосте: `http://127.0.0.1:9000`.

TLS: положите `fullchain.pem` и `privkey.pem` в `deploy/compose/production/certs` и добавьте `--profile tls`.

Копии и остальной список выхода в бой: [production-checklist.md](production-checklist.md).

Образы по умолчанию лежат на GitVerse: `gitverse.ru/direktorbani/datasafe-storage-server:v1.7.1` и `gitverse.ru/direktorbani/datasafe-console:v1.7.1` (реестр публичный). Подробности про реестры GitVerse: [gitverse-mirror.md](gitverse-mirror.md). Свой реестр или локально собранный образ задаются через `DATASAFE_SERVER_IMAGE` и `DATASAFE_CONSOLE_IMAGE` в `.env`.

Интерфейс консоли доступен на русском и английском. Kafka и Vault этот файл не запускает.
