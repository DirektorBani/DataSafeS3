**[English](../en/monitoring.md)** | Русский

# Мониторинг

![Monitoring](../../images/screenshots/monitoring.png)

## Стек

| Компонент | URL | Роль |
|-----------|-----|------|
| Prometheus | http://localhost:9090 | Scrape `/metrics` |
| Grafana | http://localhost:3000 | Дашборд **DataSafeS3 Overview** |

## Ключевые метрики

- HTTP RPS и latency
- Байты хранилища, число buckets/объектов
- S3 read/write операции
- Глубина очереди репликации
- CPU, память, диск хоста (Linux)

## Консоль

Страница Usage — потребление по пользователям. Gateway — здоровье репликации.

## Полное руководство

[Мониторинг и БД](../../ru/user-guide/07-monitoring-i-bazy.md) · [Руководство по эксплуатации](../../operations-guide/ru/monitoring.md)
