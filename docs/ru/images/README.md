# Изображения

English: [images/README.md](../../images/README.md).

## Снимки интерфейса

Кадры с работающего DataSafeS3:

| Файл | Экран |
|------|--------|
| [screenshots/dashboard.png](../../images/screenshots/dashboard.png) | Обзор |
| [screenshots/buckets.png](../../images/screenshots/buckets.png) | Список бакетов |
| [screenshots/object-browser.png](../../images/screenshots/object-browser.png) | Браузер объектов |
| [screenshots/tenants.png](../../images/screenshots/tenants.png) | Арендаторы |
| [screenshots/users.png](../../images/screenshots/users.png) | Пользователи |
| [screenshots/settings.png](../../images/screenshots/settings.png) | Настройки |
| [screenshots/gateway.png](../../images/screenshots/gateway.png) | Шлюз репликации |
| [screenshots/activity.png](../../images/screenshots/activity.png) | Журнал действий |
| [screenshots/monitoring.png](../../images/screenshots/monitoring.png) | Grafana, обзор DataSafeS3 |

Повторная съёмка консоли: `node scripts/capture-screenshots.mjs`. Набор для обложки: `cd scripts/screenshots && npm run capture`.

**Последняя съёмка:** 2026-07-06 — дашборд мониторинга (вручную, рабочая Grafana); снимки консоли 2026-06-19 (Playwright, `localhost:8080`).

Интерфейс на этих кадрах может показывать английские подписи того прогона. Текущая консоль доступна на русском и английском.

## Диаграммы

Диаграммы архитектуры — Mermaid в Markdown. См. [diagrams/README.md](../diagrams/README.md).

## Прежний каталог

Более ранние кадры лежат в [user-guide/images/](../../user-guide/images/): вход, профиль MFA, объект в бакете. Для README и обложки предпочтителен каталог `screenshots/`. Руководство пользователя использует оба.
