**[English](../en/setup-wizard.md)** | Русский

# Мастер первичной настройки

При первом входе администратора после чистой установки DataSafeS3 перенаправляет на `/setup`, пока начальная конфигурация не завершена.

## Этапы

1. **Смена пароля** — обязательна перед остальными шагами
2. **Приветствие** — обзор опционального внешнего S3
3. **Внешний S3** (опционально) — подключение external S3 для репликации Gateway
4. **Завершение** — пропустить S3 или сохранить проверенное подключение

## API статус

```http
GET /api/v1/setup/status
```

Флаги: `needs_setup`, `needs_password_change`, `initial_setup_completed`.

## Пропуск внешнего S3

После смены пароля:

```http
POST /api/v1/setup/complete
Authorization: Bearer <token>
```

## Настройка внешнего S3

```http
POST /api/v1/setup/s3/save
```

Требуется успешный тест подключения. См. [Настройка S3](s3-configuration.md).

## Спецификация

Полное ТЗ: [../../ru/specs/initial-setup-wizard-tz.md](../../ru/specs/initial-setup-wizard-tz.md)
