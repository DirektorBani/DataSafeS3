**[English](../en/audit.md)** | Русский

# Аудит и журнал активности

![Activity](../../images/screenshots/activity.png)

DataSafeS3 записывает административные и data-plane действия в журнал **Activity**.

## События

- CRUD пользователей/buckets/объектов
- Изменения настроек, входы
- Создание share links
- Триггеры репликации Gateway

## Консоль

**Администрирование → Activity** — фильтр по действию, пользователю, ресурсу.

## API

```http
GET /api/v1/activity?limit=100
```

## Внешнее логирование

Дублирование JSON-логов в Syslog, Loki, Elasticsearch, Webhook — **Settings → External logging**.

См. [operations guide — мониторинг](../../operations-guide/ru/monitoring.md).
