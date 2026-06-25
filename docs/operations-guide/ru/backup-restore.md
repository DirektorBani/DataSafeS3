**[English](../en/backup-restore.md)** | Русский

# Backup и restore

## Что резервировать

| Актив | Путь / метод |
|-------|--------------|
| Данные объектов | `STORAGE_DATA_DIR/objects/` |
| Метаданные BoltDB | `STORAGE_DATA_DIR/metadata.db` |
| Метаданные PostgreSQL | `pg_dump` базы `STORAGE_POSTGRES_DB` |
| Конфигурация | `.env`, Kubernetes Secrets |

## Процедура backup

```bash
# Остановить записи (опционально, для консистентного снимка)
docker compose stop storage-server

# Копия data volume
tar czf datasafe-backup-$(date +%F).tar.gz ./data/

# PostgreSQL
docker exec datasafe-postgres-1 pg_dump -U datasafe datasafe > metadata.sql

docker compose start storage-server
```

## Восстановление

1. Развернуть чистый DataSafeS3 с теми же `STORAGE_*`
2. Восстановить `objects/` и `metadata.db` ИЛИ импорт PostgreSQL
3. Запустить `storage-server`, проверить buckets/objects

## Репликация Gateway

[Репликация Gateway](../../administrator-guide/ru/replication.md) как непрерывная off-site копия на удалённой стороне/AWS.

## Ротация SSE master key {#sse-master-key-rotation}

При заданном `STORAGE_SSE_MASTER_KEY` шифрование SSE-S3 выводит ключи объектов из master secret.

1. **Подготовка** — убедитесь в работоспособности backup; запланируйте окно обслуживания.
2. **Перешифрование** — автоматической смены ключа на месте в Community Edition нет. Ротация:
   - новый ключ на новом инстансе или после полного backup;
   - копирование объектов наружу и обратно (или restore), чтобы записать с новым ключом.
3. **Обновить env** — новый `STORAGE_SSE_MASTER_KEY` в `.env`, Kubernetes Secrets или Helm.
4. **Проверка** — тестовый upload/get; журнал активности.
5. **Удалить старый ключ** из хранилищ секретов после валидации.

Сочетайте с [процедурой backup](#backup-procedure) и `STORAGE_STRICT_SECRETS=true` для проверки дефолтных credentials при старте.
