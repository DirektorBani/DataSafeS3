**[English](../en/monitoring.md)** | Русский

# Мониторинг

![Grafana — дашборд DataSafeS3 Overview](../../images/screenshots/monitoring.png)

Дашборд **DataSafeS3 Overview** состоит из трёх рядов панелей:

| Ряд | Панели |
|-----|--------|
| **HTTP / S3** | RPS, число бакетов и объектов, объём хранилища, p95 latency, HTTP-ошибки, топ бакетов по размеру |
| **Cluster (summary)** | Overall status, healthy/offline, HA is-leader |
| **Host** | Занятость диска %, CPU load (1m), память %, сеть, lag репликации PostgreSQL, ёмкость диска |

Таблица UP/DOWN по нодам и erasure: **DataSafeS3 Cluster Status** (`datasafe-cluster`).

## Стек

| Компонент | URL | Роль |
|-----------|-----|------|
| Prometheus | http://localhost:9090 | Scrape `/metrics` (+ опциональный `file_sd` нод) |
| Grafana | http://localhost:3000 | Overview + Cluster Status + Buckets |

## Ключевые метрики

- HTTP RPS и latency
- Байты хранилища, число buckets/объектов
- S3 read/write операции
- Здоровье нод кластера (`datasafe_cluster_node_up`, overall status)
- Глубина очереди репликации
- CPU, память, диск хоста (Linux)

## Консоль

Страница Usage — потребление по пользователям. Gateway — здоровье репликации.

## Полное руководство

[Мониторинг и БД](../../ru/user-guide/07-monitoring-i-bazy.md) · [Руководство по эксплуатации](../../operations-guide/ru/monitoring.md)
