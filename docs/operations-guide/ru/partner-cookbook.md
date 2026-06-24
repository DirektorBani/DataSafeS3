# Cookbook интеграций для партнёров

**[English](../en/partner-cookbook.md)** | Русский

Рецепты для backup, Kubernetes и SIEM с DataSafeS3 Community Edition.

## Velero (backup в Kubernetes)

Настройте BackupStorageLocation с S3-совместимым URL DataSafeS3 (`s3Url`, `s3ForcePathStyle: "true"`). Ключи доступа — в Admin → Users.

## restic

```bash
export RESTIC_REPOSITORY=s3:https://datasafe.example.com/my-backup-bucket
restic backup /data/to/archive
```

## SIEM

| Приёмник | Настройка |
|----------|-----------|
| Webhook | Admin → Webhooks |
| NATS | `STORAGE_NATS_URL=nats://nats:4222`, профиль compose `nats` |

## STS (ограниченная загрузка)

`POST /api/v1/sts/assume-role` вызывайте с токеном **того пользователя**, для которого нужен scoped S3-доступ (`ds_*` или JWT сессии). Выданные credentials **привязаны к этому пользователю**.

Используйте `session_token` в заголовке `X-Amz-Security-Token` при SigV4-запросах к S3.

## Примеры SDK

`examples/go`, `examples/python`, `examples/js`, `examples/extension-hook/`.
