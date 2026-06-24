**[English](../en/lifecycle.md)** | Русский

# Политики lifecycle

Автоматическое истечение срока и правила перехода для объектов в bucket.

## Поток

```mermaid
flowchart LR
  upload[Загрузка объекта]
  rules[Правила lifecycle в метаданных]
  job[Фоновая оценка]
  trash[Soft delete / expire]
  upload --> rules
  rules --> job
  job --> trash
```

## Настройка

Настройки bucket → **Lifecycle** — правила (prefix, дни, действие).

## Связанное

- **Корзина** — удалённые объекты в `.datasafe-trash`
- **Блокировка объектов (Object Lock)** — юридическое удержание и срок хранения (compliance)

## Полное руководство

[Главная и бакеты](../../ru/user-guide/02-dashbord-i-bakety.md)
