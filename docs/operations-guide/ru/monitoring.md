**[English](../en/monitoring.md)** | Русский

# Эксплуатационный мониторинг

![Grafana — дашборд DataSafeS3 Overview](../../images/screenshots/common/monitoring.png)

Панели обзора: **HTTP / S3** (RPS, счётчики, объём, latency, ошибки, топ бакетов), **Cluster (summary)** (общий статус, healthy/offline, HA leader), **Host** (диск, CPU, память, сеть, lag Postgres), **Event delivery** (очередь, скорость доставки, задержка, перезапуски моста) и **KMS** (доступность Vault по узлам, латентность запросов, кэш DEK, фоновые задачи). Ряды KMS есть только пока включён Vault Transit, кроме `datasafe_kms_jobs_objects_total`, который появляется после первого обработанного объекта, и счётчиков классификации 1.5.0, которые регистрируются при старте процесса.

Полный статус нод и erasure: дашборд **DataSafeS3 Cluster Status** (`deploy/docker/grafana/dashboards/datasafe-cluster.json`, uid `datasafe-cluster`).

## Prometheus

- Scrape: `storage-server:9000/metrics`
- Конфиг: `deploy/docker/prometheus.yml`
- **Ноды кластера (опционально):** правьте `deploy/docker/prometheus/targets/cluster-nodes.json` (см. `cluster-nodes.example.json`) — адреса `host:9000` дополнительных нод. В поставке список целей пустой, поэтому одиночный узел снимает только job `storage-server`. Compose монтирует каталог в Prometheus (`file_sd`).
- **v1.1.0+:** при `STORAGE_METRICS_TOKEN` `GET /metrics` требует этот bearer. **v1.4.1:** корневой установщик генерирует токен, а Prometheus в Compose подставляет его, если переменная непустая (`# metrics_auth` в `deploy/docker/prometheus.yml`). См. [обновление до v1.4.1](upgrade.md#обновление-до-v141).
- **Консоль:** без bearer дашборд может показывать нули — используйте Grafana/Prometheus.

## Grafana

- URL: http://localhost:3000 (по умолчанию `admin`/`admin`)
- Дашборды:
  - **Overview** — `datasafe-overview`
  - **Cluster Status** — `datasafe-cluster` (таблица UP/DOWN, overall, HA, erasure, heal, PG lag)
  - **Buckets** — `datasafe-buckets`
- В Compose зафиксированы образы `grafana/grafana:13.2.3` и `prom/prometheus:v3.15.0`. Задача CI `event-observability` запускает `promtool check rules` для файла алертов и поднимает этот образ Grafana, затем читает дашборд `datasafe-overview` через HTTP API.

## Доставка событий

Ряд **Event delivery** на дашборде обзора (`deploy/docker/grafana/dashboards/datasafe-overview.json` и копия в чарте Helm) строится по метрикам диспетчера:

| Панель | Запрос |
|--------|--------|
| Outbox pending | `datasafe_events_outbox_pending` — сколько событий ждёт доставки |
| Delivery rates | `rate` успешных, неудачных и окончательно не доставленных событий по получателю, и сброшенных по причине (`pending_cap` — это переполнение очереди, не имя получателя) |
| Delivery lag | `histogram_quantile` 0.50 / 0.95 / 0.99 задержки от записи в очередь до успешной доставки |
| Sink degraded | `datasafe_events_sink_degraded` равен 1, пока получатель в ошибке, в том числе до первого успешного соединения |
| Event bridge restarts | `datasafe_events_bridge_restarts_total` — сколько раз процесс перезапустил дочерний мост Kafka |

Пример правил: `deploy/docker/prometheus/alerts.yml`. Compose монтирует файл, `deploy/docker/prometheus.yml` подключает его через `rule_files`. Порог очереди меняется в этом файле.

| Алерт | Когда срабатывает |
|-------|-------------------|
| `DataSafeEventOutboxPending` | В очереди больше 1000 событий, или счётчик очереди вырос за 10 минут и события ещё ждут, и это длится 10 минут |
| `DataSafeEventSinkDegraded` | У любого получателя `datasafe_events_sink_degraded` равен 1 в течение 5 минут |
| `DataSafeEventDeadIncreasing` | `datasafe_events_dead_total` вырос за последние 10 минут |
| `DataSafeEventDroppedIncreasing` | `datasafe_events_dropped_total{reason="pending_cap"}` вырос за последние 10 минут |
| `DataSafeKMSUnavailable` | `datasafe_kms_available` равен 0 дольше 1 минуты. Ряд публикуется только пока включён Vault Transit |

## KMS

Ряд **KMS** на том же дашборде обзора. Узел, на котором Vault не включён, не публикует `datasafe_kms_available`, гистограмму запросов и счётчики кэша. Снимайте метрики с каждого узла, чтобы `instance` был этим узлом.

| Панель | Запрос |
|--------|--------|
| KMS available | `datasafe_kms_available` по `instance`. 1 — последняя проверка Vault на этом узле |
| KMS latency | p50 и p95 `datasafe_kms_request_duration_seconds` по `instance`, `op` и `provider` |
| DEK cache | `rate` счётчиков `datasafe_kms_dek_cache_hits_total` и `datasafe_kms_dek_cache_misses_total` |
| KMS jobs | `rate` `datasafe_kms_jobs_objects_total` по `instance`, `job` и `result` |

В журнале Activity строка `events_sink_degraded` пишется при первой настоящей ошибке получателя, а `events_sink_recovered` — когда соединение снова удалось. Повторы той же ошибки новых строк не добавляют. Фраза `waiting for first successful connection` — причина статуса до первой проверки; в Activity она не попадает. `events_outbox_dropped` — переполнение очереди. Эти действия есть в фильтре Activity в консоли. `POST /api/v1/events/test` только ставит проверочное событие в очередь.

## Метрики кластера (Wave 2+)

Публикует процесс storage с cluster monitor (probe `/healthz` настроенных нод):

| Метрика | Смысл |
|---------|--------|
| `datasafe_cluster_overall_status` | 2=healthy, 1=degraded, 0=offline |
| `datasafe_cluster_nodes_total` / `_healthy` / `_offline` | Счётчики нод |
| `datasafe_cluster_node_up{node_id,address,role}` | 1 если нода жива |
| `datasafe_cluster_node_status{…,status}` | Код статуса ноды |
| `datasafe_ha_enabled` / `datasafe_ha_is_leader` | HA election на этом процессе |
| `datasafe_events_outbox_pending` | Строки уведомлений, ожидающие доставки |
| `datasafe_events_delivered_total{sink}` | Успешные доставки sink |
| `datasafe_events_failed_total{sink,reason}` | Неудачные попытки доставки |
| `datasafe_events_dead_total{sink}` | Sink, исчерпавшие повторы |
| `datasafe_events_delivery_lag_seconds{sink}` | От вставки до успешной доставки |
| `datasafe_events_dropped_total{reason}` | Строки, сброшенные, чтобы запись объектов не блокировалась |
| `datasafe_events_sink_degraded{sink}` | 1, пока этот получатель в ошибке |
| `datasafe_events_bridge_restarts_total` | Перезапуски дочернего моста Kafka |

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
