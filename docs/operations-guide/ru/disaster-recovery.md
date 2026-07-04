**[English](../en/disaster-recovery.md)** | Русский

# Аварийное восстановление

## RPO / RTO (HA v2)

| Стратегия | RPO (лаб.) | RTO (лаб.) | Сложность |
|-----------|------------|------------|-----------|
| Ежедневный tarball | 24ч | Часы | Низкая |
| Postgres streaming + erasure | Секунды (lag метаданных) | ≤ 5 мин (скрипт failover) | Средняя |
| Site replication (peer DataSafeS3) | 30–60 с async | Минуты | Средняя |
| Gateway → external S3 | Минуты | Часы | Средняя |
| Legacy shared-FS standby | = диск primary | N/A (только чтение) | **deprecated** |

**Автоматический failover** всего стека не предполагается без Patroni (или аналога). CE поставляет **скрипты**, не постоянный контроллер failover.

## Восстановление

### Postgres / метаданные

1. Promote standby (`scripts\ha\failover-metadata.ps1` или `pg_promote`).
2. Обновить `STORAGE_POSTGRES_DSN`, сбросить устаревший leader lock при необходимости.
3. Перезапустить storage-server; проверить `/healthz`.

### Erasure objects

1. Восстановить **все** пути из `STORAGE_ERASURE_DATA_PATHS`.
2. Запустить с `STORAGE_OBJECT_BACKEND=erasure`; дождаться `erasure_degraded=false`.

### Переключение на Site B

1. Promote Postgres на Site B (отдельный деплой).
2. DNS/Ingress на Site B.
3. Проверить GET объекта и `GET /api/v1/site-replication/status`.

Ежеквартальный DR drill: restore в изолированное окружение, `scripts\ha\test-ha-cluster.ps1`.
