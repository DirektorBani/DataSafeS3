Русский | **[English](../en/migrate-from-minio.md)**

# Миграция с MinIO на DataSafeS3

Runbook для переноса объектов с **MinIO-совместимого** S3 endpoint на **DataSafeS3** Community Edition.

> **Честно:** это не drop-in замена бинарника и не заявление о 100% совместимости API MinIO. Байты объектов переносятся стандартными S3 sync-инструментами. Пользователи IAM MinIO, группы и server-side policies **не** импортируются автоматически — перенесите их в пользователей, teams, tenants и bucket policies DataSafe.

Архитектура: [ADR 0001](../../architecture/adr/0001-migration-kit.md) · Чеклист: `internal/migrate`.

---

## 1. Когда использовать

| Сценарий | Подход |
|----------|--------|
| Есть бакеты MinIO с данными | **Параллельный cutover** (рекомендуется) |
| Временный гибрид | Опционально [Gateway](../../ru/context/gateway.md) |
| Пустой target / greenfield | Sync не нужен — только бакеты и ключи |

## 2. Предварительные условия

- DataSafeS3 запущен; `GET /healthz` OK
- Для production рекомендуется **PostgreSQL** ([первый запуск](../../getting-started/ru/first-run.md))
- Сеть: хост оператора достучится до MinIO (source) и DataSafe (target)
- Инструменты: [rclone](https://rclone.org/) **или** AWS CLI v2
- На DataSafe: бакеты созданы, S3-ключи выданы (Admin → Keys)

## 3. Что переносится / что нет

| Переносится через S3 sync | Не переносится автоматически |
|---------------------------|------------------------------|
| Байты объектов | IAM users / groups MinIO |
| Обычные user-metadata заголовки S3 PUT | Policy JSON MinIO as-is (нужен remap principals) |
| Завершённые multipart (как финальные объекты) | Конфиг сервера MinIO, закладки консоли |
| | Object Lock / versioning **конфиг сервера** — включите заново на DataSafe |

## 4. Таблица соответствия

| MinIO | DataSafeS3 |
|-------|------------|
| Access Key / Secret | S3 access keys (Admin → Keys) |
| Bucket | Bucket (лучше то же имя) |
| Bucket policy JSON | Bucket policy UI / JSON (remap principals) |
| Users / groups | Users + [Teams](../../administrator-guide/ru/teams.md) + Tenants |
| Server-side replication | [Gateway](../../administrator-guide/ru/replication.md) или [Trusted clusters](./trusted-clusters.md) |
| Console | Веб-консоль DataSafe |

## 5. Конфигурация rclone

```ini
[minio]
type = s3
provider = Minio
env_auth = false
access_key_id = SOURCE_ACCESS_KEY
secret_access_key = SOURCE_SECRET_KEY
endpoint = http://minio.example.com:9000
acl = private

[datasafe]
type = s3
provider = Other
env_auth = false
access_key_id = DATASAFE_ACCESS_KEY
secret_access_key = DATASAFE_SECRET_KEY
endpoint = http://datasafe.example.com:9000
acl = private
force_path_style = true
```

Пример файла: [examples/rclone-minio-to-datasafe.conf](../en/examples/rclone-minio-to-datasafe.conf).

### Sync

```bash
rclone sync minio:my-bucket datasafe:my-bucket --checksum --progress
```

Повторите для каждого бакета. Сверка объёма: `rclone size minio:my-bucket` vs `rclone size datasafe:my-bucket`.

## 6. Альтернатива aws-cli

```bash
aws --endpoint-url http://minio.example.com:9000 s3 sync s3://my-bucket /tmp/my-bucket
aws --endpoint-url http://datasafe.example.com:9000 s3 sync /tmp/my-bucket s3://my-bucket
```

## 7. Чеклист cutover

Печать:

```bash
go run ./cmd/storage-cli migrate checklist minio
```

Тот же текст: `internal/migrate.ChecklistMarkdown()`:

1. Заморозить запись на MinIO (окно обслуживания)
2. Финальный `rclone sync … --checksum`
3. Smoke: `pwsh -File scripts/migrate/minio-cutover-smoke.ps1` — см. [scripts/migrate/README.md](../../../scripts/migrate/README.md)
4. Переключить приложения / DNS на endpoint DataSafe
5. Разморозить запись **только** на DataSafe
6. Держать MinIO read-only для rollback (например 7–14 дней)
7. Не удалять source до PASS smoke и стабильной работы приложений

## 8. Проверка (smoke)

```powershell
pwsh -File scripts/migrate/minio-cutover-smoke.ps1 -DryRun `
  -SourceEndpoint http://minio:9000 -DestEndpoint http://127.0.0.1:9000 -Bucket my-bucket

$env:SOURCE_SECRET = "…"
$env:DEST_SECRET = "…"
pwsh -File scripts/migrate/minio-cutover-smoke.ps1 `
  -SourceEndpoint http://minio:9000 -SourceKey AKIA… `
  -DestEndpoint http://127.0.0.1:9000 -DestKey datasafe `
  -Bucket my-bucket -SampleSize 20
```

## 9. После cutover

- Снова включите **Object Lock** / **versioning** на бакетах DataSafe, если нужны immutable backup — следуйте [immutable backup](../../use-cases/ru/immutable-backup.md) ([ADR 0003](../../architecture/adr/0003-immutable-backup-path.md)).
- LDAP/OIDC — [onboarding](../../getting-started/ru/onboarding.md).
- Off-site: [Gateway](../../administrator-guide/ru/replication.md).

## 10. Rollback

1. Остановить запись в DataSafe.
2. Вернуть приложения на endpoint MinIO.
3. Копия на DataSafe считается устаревшей до нового sync.

## 11. FAQ

**Можно ли автоматически импортировать пользователей MinIO?**  
Нет. Создайте пользователей/teams/tenants DataSafe и новые S3-ключи.

**Нужен ли Gateway?**  
Нет. Предпочтителен параллельный sync + cutover.

**Trusted clusters?**  
Когда **оба** сайта — DataSafeS3 — [trusted-clusters.md](./trusted-clusters.md). Для MinIO как source используйте этот гайд (rclone/aws).
