# ADR 0004: задания инвентаризации

English: [0004-inventory-jobs.md](../../../architecture/adr/0004-inventory-jobs.md).

**Статус:** принято частично — в Community Evidence Pack есть ручной CSV через Admin API  
**Дата:** 2026-07-10 (обновлено 2026-08-20)

## Контекст

Оператору нужна выгрузка ёмкости и списка для соответствия требованиям и планирования, без биллинга MSP.

## Решение

`internal/inventory.InventoryJob` и ручные задания Admin API пишут CSV (скачивание и необязательный бакет назначения). Работает на **Bolt и PostgreSQL**. Реестр заданий в процессе, перезапуск его не сохраняет. Потолок — 100 000 объектов на задание.

Планировщик на лидере и Parquet остаются будущей работой. Совместимый с AWS `GetBucketInventoryConfiguration` в этот срез явно не входит.

## Где лежит код

Пакет `internal/inventory` и `internal/api/inventory_handlers.go`. Чеклист для оператора: `docs/use-cases/*/governance-evidence.md`.
