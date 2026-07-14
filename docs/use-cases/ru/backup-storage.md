**[English](../en/backup-storage.md)** | Русский

# Репозиторий резервных копий

## Проблема

Инструменты резервного копирования (Veeam, restic, Velero, скрипты) нуждаются в надёжной S3-совместимой цели на контролируемой инфраструктуре.

## Решение

Используйте DataSafeS3 как основную площадку для backup:

```mermaid
flowchart LR
  backup[ПО резервного копирования]
  ds[DataSafeS3 S3]
  gw[Gateway опционально]
  remote[Внешняя S3-площадка]
  backup -->|S3 API| ds
  ds -->|async replication| gw --> remote
```

1. Production с PostgreSQL ([первый запуск](../../getting-started/ru/first-run.md))
2. Отдельные бакеты на workload
3. S3-ключи с минимальными правами на каждую задачу
4. Опционально: [репликация Gateway](../../administrator-guide/ru/replication.md)
5. [Lifecycle](../../administrator-guide/ru/lifecycle.md) для истечения старых точек
6. Для ransomware-resistant зоны следуйте [immutable backup](immutable-backup.md) (Object Lock + versioning)

## Результат

Предсказуемый self-hosted backup target с опциональной георепликацией через Gateway — под вашими политиками retention и доступа. Для WORM-path см. [immutable backup](immutable-backup.md).

## Миграция существующего MinIO backup target

Если backup уже пишется в MinIO (или другой S3-compatible store), перенесите бакеты по [гайду миграции MinIO → DataSafeS3](../../operations-guide/ru/migrate-from-minio.md), затем проверьте Object Lock / versioning на стороне DataSafe.

## Проверенный скрипт

Smoke: создание бакета и round-trip объекта на работающем стеке:

```powershell
.\scripts\reference-arch\backup-restore.ps1
```

Ожидается `PASS reference-arch backup-restore smoke`. См. [руководство backup & restore](../../operations-guide/ru/backup-restore.md).
