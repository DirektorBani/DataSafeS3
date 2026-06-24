**[English](../en/scaling.md)** | Русский

# Масштабирование

Community Edition DataSafeS3 — **single-node по умолчанию**. Ниже — что доступно сегодня без обещаний автоматического HA.

## Single-node по умолчанию

| Подход | Статус | Примечания |
|--------|--------|------------|
| Один `storage-server` + BoltDB/Postgres | **Реализовано** | Базовая модель |
| Вертикальное масштабирование | **Реализовано** | Основной путь сегодня |
| Gateway-репликация во внешний S3 | **Реализовано** | Копии off-site, не active-active HA |
| Federation (multi-cluster) | **Реализовано** | GetObject + ListObjectsV2 proxy через peer |
| Read replicas Postgres | **Реализовано** | `STORAGE_POSTGRES_READ_REPLICA_DSN` для list/search/count |
| Multi-AZ / erasure coding | **Частично** | Erasure 2+1 MVP в `internal/storage/erasure/` |

Не предполагайте автоматический failover без сверки с [архитектурой](../../ru/context/architecture.md).

## Вертикальное

- Больше CPU/RAM для `storage-server`
- Быстрее/больше диск для `STORAGE_DATA_DIR`
- PostgreSQL для метаданных при высокой конкуренции

## HA метаданных PostgreSQL (active-passive)

Поддерживается **ручная** streaming-репликация PostgreSQL. Автоматический failover не входит в Community Edition.

### Primary + standby

1. PostgreSQL 15+ на primary и standby.
2. На primary создайте пользователя репликации и включите `wal_level=replica`, `max_wal_senders`, `hot_standby=on`.
3. Настройте `pg_hba.conf` для replication-подключения.
4. На standby — base backup, `standby.signal`, `primary_conninfo` (стандартная схема PostgreSQL).
5. Primary `storage-server`: `STORAGE_POSTGRES_DSN` на primary.
6. Опционально: `STORAGE_POSTGRES_READ_REPLICA_DSN` на primary для маршрутизации **list buckets** / **list objects** на standby.

### Здоровье и lag

- `GET /healthz`: `postgres_ok`, `postgres_replication_lag_seconds`.
- Алерт при превышении допустимого lag (панель Grafana в комплекте).

### Ручной failover (метаданные)

1. Остановите запись: `STORAGE_READ_ONLY=true` на старом primary или остановите процесс.
2. Повысьте standby: `pg_ctl promote` / `SELECT pg_promote();`.
3. Обновите `STORAGE_POSTGRES_DSN` на всех `storage-server`.
4. Перезапустите сервис, проверьте `/healthz` и вход в консоль.
5. При необходимости пересоберите цепочку репликации.

См. [disaster-recovery](./disaster-recovery.md).

## Read-only standby storage-server

`STORAGE_READ_ONLY=true` — мутирующие API возвращают **503** с `Retry-After`; GET/List/Head доступны для DR. Пример: `docker-compose.ha.yml`. **Community Edition — полный HA** (скрипты failover, DR drill, Helm `values-ha.yaml`): [эталонное развёртывание](./reference-deployment-2node.md).

## Горизонтальные варианты

| Подход | Статус | Примечания |
|--------|--------|------------|
| **Репликация Gateway** | Реализовано | Копии во внешний S3 |
| **Federation (MVP)** | Реализовано | [federation](../../ru/user-guide/08-federation-i-cluster.md) |
| **Read replicas** | Реализовано | `STORAGE_POSTGRES_READ_REPLICA_DSN` |

## Kubernetes

Helm: лимиты, PDB, `values-production.yaml`. [deploy/helm/datasafe/README.md](../../../deploy/helm/datasafe/README.md).

Бенчмарки: [performance-benchmarks](../../testing/performance-benchmarks.md).
