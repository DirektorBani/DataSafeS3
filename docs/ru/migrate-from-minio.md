# Перенос данных с MinIO

English: [migrate-from-minio.md](../migrate-from-minio.md).

Два способа уйти с бакета MinIO. Скопировать объекты клиентом или взять встроенный копир, если нужны версии, Object Lock и IAM.

Клиентское копирование обычно делают первым. Встроенный копир описан в [руководстве по эксплуатации](../operations-guide/ru/migrate-from-minio.md).

## mc mirror

```bash
mc alias set src http://minio:9000 MINIO_ROOT_USER MINIO_ROOT_PASSWORD
mc alias set dst http://datasafe:9000 ACCESS_KEY SECRET_KEY --api s3v4 --path on
mc mb dst/photos
mc mirror --overwrite src/photos dst/photos
```

`mc rm --recursive` к DataSafeS3 использует DeleteObjects. На 1.7.0 это работает.

## rclone sync

```bash
export RCLONE_CONFIG_SRC_TYPE=s3
export RCLONE_CONFIG_SRC_PROVIDER=Minio
export RCLONE_CONFIG_SRC_ENDPOINT=http://minio:9000
export RCLONE_CONFIG_SRC_ACCESS_KEY_ID=MINIO_ROOT_USER
export RCLONE_CONFIG_SRC_SECRET_ACCESS_KEY=MINIO_ROOT_PASSWORD

export RCLONE_CONFIG_DST_TYPE=s3
export RCLONE_CONFIG_DST_PROVIDER=Other
export RCLONE_CONFIG_DST_ENDPOINT=http://datasafe:9000
export RCLONE_CONFIG_DST_ACCESS_KEY_ID=ACCESS_KEY
export RCLONE_CONFIG_DST_SECRET_ACCESS_KEY=SECRET_KEY
export RCLONE_CONFIG_DST_FORCE_PATH_STYLE=true
export RCLONE_CONFIG_DST_REGION=us-east-1

rclone sync src:photos dst:photos
```

Нужен path-style. Виртуальные хосты бакетов не обслуживаются.

## Чего клиентское копирование не переносит

| MinIO | После mc mirror или rclone sync |
|-------|----------------------------------|
| Текущие байты, имена и те пользовательские метаданные, что инструмент шлёт | копируются |
| Версии и маркеры удаления | не копируются. Нужен `storage-cli migrate from-minio` |
| Object Lock, срок хранения, юридическое удержание | этими инструментами не копируются |
| Политика бакета и пользователи IAM | не копируются. `storage-cli migrate from-minio iam` заводит пользователей, группы и политики, которые сервер умеет применить. LDAP и OIDC сообщаются и не импортируются |
| Шифрование бакета по умолчанию | не копируется. Включите шифрование на бакете DataSafeS3 после копирования |
| Правила перехода жизненного цикла | не сохраняются. Истечение по префиксу в днях принимается. Остальные правила жизненного цикла отклоняются |

## Похожие настройки

| MinIO | DataSafeS3 |
|-------|------------|
| `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` | `STORAGE_ACCESS_KEY` / `STORAGE_SECRET_KEY` для S3. Администратор консоли — `STORAGE_ADMIN_USER` / `STORAGE_ADMIN_PASSWORD` |
| `MINIO_REGION` или регион площадки | `STORAGE_REGION` (по умолчанию `us-east-1`) |
| Каталог данных | `STORAGE_DATA_DIR` (в контейнере по умолчанию `/data`) |
| Распределённый MinIO | Один узел использует Bolt. Несколько узлов делят Postgres (`STORAGE_METADATA_BACKEND=postgres`, `STORAGE_HA_ENABLED=true`). Набор erasure MinIO из них не получается |
| Prometheus `/minio/v2/metrics/cluster` | `GET /metrics` с `STORAGE_METRICS_TOKEN`, если переменная задана. `STORAGE_DEV` оставьте пустым или `false` |

Направьте aws-cli, mc и rclone на `http://хост:9000` с `--addressing-style path` (у mc — `--path on`). Матрица: [compatibility.md](compatibility.md).
