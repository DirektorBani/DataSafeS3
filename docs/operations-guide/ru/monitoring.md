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

### Политика исходящих URL (v1.0.2+)

URL sink'ов, webhooks и hook-test проверяются на SSRF (`internal/security/urlpolicy`):

- **Production** (`STORAGE_DEV=false`): только публичные `https://` (private IP, `localhost`, metadata IP запрещены).
- **Локальная разработка**: `STORAGE_DEV=true` или `STORAGE_OUTBOUND_HTTP_ALLOW=true` для `http://127.0.0.1` / `host.docker.internal`.
- Невалидный URL → `400` с `outbound url not allowed: …` при сохранении настроек или тесте hook.

Полное руководство: [../../ru/user-guide/07-monitoring-i-bazy.md](../../ru/user-guide/07-monitoring-i-bazy.md)
