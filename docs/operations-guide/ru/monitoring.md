**[English](../en/monitoring.md)** | Русский

# Эксплуатационный мониторинг

![Monitoring](../../images/screenshots/monitoring.png)

## Prometheus

- Scrape: `storage-server:9000/metrics`
- Конфиг: `deploy/docker/prometheus.yml`

## Grafana

- URL: http://localhost:3000 (по умолчанию `admin`/`admin`)
- Дашборд: **DataSafeS3 Overview** (`deploy/docker/grafana/dashboards/datasafe-overview.json`)

## Рекомендуемые алерты

| Алерт | Метрика |
|-------|---------|
| Диск > 85% | node filesystem |
| Очередь репликации | `datasafe_replication_queue_depth` |
| 5xx rate | HTTP metrics |
| Всплеск auth failures | login counter |

## Внешнее логирование

Пересылка JSON-логов в Loki/Elasticsearch для корреляции с audit.

Полное руководство: [../../ru/user-guide/07-monitoring-i-bazy.md](../../ru/user-guide/07-monitoring-i-bazy.md)
