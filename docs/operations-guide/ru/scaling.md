**[English](../en/scaling.md)** | Русский

# Масштабирование

Community Edition DataSafeS3 — **single-node по умолчанию**. Ниже — что доступно сегодня без обещаний автоматического HA.

## Single-node по умолчанию

| Подход | Статус | Примечания |
|--------|--------|------------|
| Один `storage-server` + BoltDB/Postgres | **Реализовано** | Базовая модель |
| Вертикальное масштабирование | **Реализовано** | Основной путь сегодня |
| Gateway-репликация во внешний S3 | **Реализовано** | Копии off-site, не active-active HA |
| Federation (multi-cluster) | **Реализовано** | GetObject/List proxy; peer привязан к **кластеру** — [trusted clusters](./trusted-clusters.md) |
| Trusted cluster pairing + repl | **Lab (v1.1.0)** | mTLS, join token — [руководство](./trusted-clusters.md) |
| Read replicas Postgres | **Реализовано** | `STORAGE_POSTGRES_READ_REPLICA_DSN` для list/search/count |
| Multi-AZ / erasure coding | **Lab foundation (v1.1)** | `STORAGE_OBJECT_BACKEND=erasure`; локальная lab-проверка, не production multi-AZ |

Не предполагайте автоматический failover или production multi-AZ durability без сверки с [архитектурой](../../ru/context/architecture.md). HA v2 в v1.1.0 — **CE lab foundation**, не production Patroni/auto-failover cluster.

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

`STORAGE_READ_ONLY=true` — мутирующие API возвращают **503** с `Retry-After`; GET/List/Head доступны для DR. Пример: `docker-compose.ha.yml`. **Community Edition — HA lab / DR tooling** (скрипты failover, DR drill, Helm `values-ha.yaml`): [эталонное развёртывание](./reference-deployment-2node.md).

## Горизонтальные варианты

| Подход | Статус | Примечания |
|--------|--------|------------|
| **Репликация Gateway** | Реализовано | Копии во внешний S3 |
| **Federation (MVP)** | Реализовано | Peer с **cluster_id**; GetObject/List — [user guide](../../ru/user-guide/08-federation-i-cluster.md) |
| **Trusted clusters** | Lab (v1.1.0) | mTLS pairing, repl — [trusted-clusters.md](./trusted-clusters.md) |
| **Read replicas** | Реализовано | `STORAGE_POSTGRES_READ_REPLICA_DSN` |
| **Erasure object backend** | Lab foundation (v1.1) | `STORAGE_OBJECT_BACKEND=erasure`, paths через `STORAGE_ERASURE_DATA_PATHS` |
| **Site replication (peer)** | Реализовано (v1.1) | DataSafeS3↔DataSafeS3 async; `STORAGE_SITE_REPLICATION_ENABLED=true` |
| **Leader lock (single writer)** | Реализовано (v1.1) | `STORAGE_HA_ENABLED=true` + таблица Postgres `ha_leader_lock` |

## Erasure object backend (v1.1)

Задаётся на `storage-server`:

| Переменная | Default | Описание |
|------------|---------|----------|
| `STORAGE_OBJECT_BACKEND` | `fs` | `fs` или `erasure` |
| `STORAGE_ERASURE_LAYOUT` | `dev` | `dev` (2+1 XOR) или `production` (4+2 Reed-Solomon) |
| `STORAGE_ERASURE_DATA_PATHS` | — | Корни shard-хранилищ через запятую (минимум data+parity paths) |
| `STORAGE_ERASURE_HEAL_INTERVAL` | `5m` | Интервал фонового heal |

Lab compose: `docker-compose.ha-erasure.yml` + `scripts/ha/test-erasure-backend.ps1`. Профиль `production` 4+2 — реализационная основа для future hardening; перед production-данными нужны собственные failure drills.

Prometheus: `datasafe_erasure_degraded_shard_sets`, `datasafe_erasure_heal_bytes_total`.

Legacy **storage-server-standby** (shared FS + read-only API) для новых HA-инсталляций считается устаревающим паттерном — предпочтительны erasure + metadata failover.

## Site replication

Gateway replication пишет во **внешний S3**. Site replication пишет в другой **DataSafeS3** peer (`POST /api/v1/site-replication/peers`). Worker включается через `STORAGE_SITE_REPLICATION_ENABLED=true`.

## Kubernetes

Helm: лимиты, PDB, `values-production.yaml`. [deploy/helm/datasafe/README.md](../../../deploy/helm/datasafe/README.md).

Бенчмарки: [performance-benchmarks](../../testing/performance-benchmarks.md).
