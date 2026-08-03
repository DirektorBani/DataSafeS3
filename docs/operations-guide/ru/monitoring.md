**[English](../en/monitoring.md)** | Русский

# Эксплуатационный мониторинг

![Grafana — дашборд DataSafeS3 Overview](../../images/screenshots/monitoring.png)

Панели обзора: **HTTP / S3** (RPS, счётчики, объём, latency, ошибки, топ бакетов), **Cluster (summary)** (общий статус, healthy/offline, HA leader) и **Host** (диск, CPU, память, сеть, lag Postgres).

Полный статус нод и erasure: дашборд **DataSafeS3 Cluster Status** (`deploy/docker/grafana/dashboards/datasafe-cluster.json`, uid `datasafe-cluster`).

## Prometheus

- Scrape: `storage-server:9000/metrics`
- Конфиг: `deploy/docker/prometheus.yml`
- **Ноды кластера (опционально):** правьте `deploy/docker/prometheus/targets/cluster-nodes.json` (см. `cluster-nodes.example.json`) — адреса `host:9000`. Compose монтирует каталог в Prometheus (`file_sd`).
- **v1.1.0+:** при `STORAGE_METRICS_TOKEN` настройте bearer в Prometheus — см. `deploy/docker/prometheus.yml` и [upgrade § v1.1.0](upgrade.md#обновление-до-v110).
- **Консоль:** без bearer дашборд может показывать нули — используйте Grafana/Prometheus.

## Grafana

- URL: http://localhost:3000 (по умолчанию `admin`/`admin`)
- Дашборды:
  - **Overview** — `datasafe-overview`
  - **Cluster Status** — `datasafe-cluster` (таблица UP/DOWN, overall, HA, erasure, heal, PG lag)
  - **Buckets** — `datasafe-buckets`

## Метрики кластера (Wave 2+)

Публикует процесс storage с cluster monitor (probe `/healthz` настроенных нод):

| Метрика | Смысл |
|---------|--------|
| `datasafe_cluster_overall_status` | 2=healthy, 1=degraded, 0=offline |
| `datasafe_cluster_nodes_total` / `_healthy` / `_offline` | Счётчики нод |
| `datasafe_cluster_node_up{node_id,address,role}` | 1 если нода жива |
| `datasafe_cluster_node_status{…,status}` | Код статуса ноды |
| `datasafe_ha_enabled` / `datasafe_ha_is_leader` | HA election на этом процессе |

Адреса нод задаются в настройках **Cluster** консоли/Admin API. Scraping `/metrics` с каждой ноды (file_sd) опционален для host-метрик по членам.

## Рекомендуемые алерты

| Алерт | Метрика |
|-------|---------|
| Диск > 85% | node filesystem / `datasafe_host_disk_used_percent` |
| Очередь Gateway | `datasafe_replication_queue_depth` |
| Lag site replication | `datasafe_site_replication_lag_seconds` |
| Очередь site replication | `datasafe_site_replication_queue_depth` |
| Degraded erasure | `datasafe_erasure_degraded_shard_sets` |
| Нода offline | `datasafe_cluster_nodes_offline > 0` или `datasafe_cluster_node_up == 0` |
| Кластер degraded | `datasafe_cluster_overall_status < 2` |
| Не leader при ожидании записи | `datasafe_ha_is_leader == 0` |
| 5xx rate | HTTP metrics |
| Всплеск auth failures | login counter |

## Метрики HA v2 (v1.1.0+)

| Метрика | Смысл |
|---------|--------|
| `datasafe_erasure_degraded_shard_sets` | Наборы shard с потерянными фрагментами |
| `datasafe_erasure_heal_bytes_total` | Объём восстановлен heal worker |
| `datasafe_site_replication_lag_seconds` | Задержка site replication |
| `datasafe_site_replication_queue_depth` | Очередь site replication |
| `datasafe_postgres_replication_lag_seconds` | Lag Postgres (backend postgres) |

На **источнике** site replication: `STORAGE_SITE_REPLICATION_ENABLED=true`.

## Внешнее логирование

Пересылка JSON-логов в Loki/Elasticsearch для корреляции с audit.

### Политика исходящих URL (v1.0.2+)

URL sink'ов, webhooks и hook-test проверяются на SSRF (`internal/security/urlpolicy`):

- **Production** (`STORAGE_DEV=false`): только публичные `https://` (private IP, `localhost`, metadata IP запрещены).
- **Локальная разработка / CI**: `STORAGE_DEV=true` (например `deploy/compose/docker-compose.audit.yml`). **`STORAGE_OUTBOUND_HTTP_ALLOW` удалена в v1.1.0.**
- Невалидный URL → `400` с `outbound url not allowed: …` при сохранении настроек или тесте hook.

Полное руководство: [../../ru/user-guide/07-monitoring-i-bazy.md](../../ru/user-guide/07-monitoring-i-bazy.md)
