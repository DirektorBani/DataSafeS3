**[English](../en/governance-evidence.md)** | Русский

# Пакет доказательств управления данными (чеклист оператора)

Соберите **операторские доказательства** для RFP / ИБ: что лежит в хранилище, кто что делал, и что сейчас защищает Object Lock.

> Это **не** сертифицированный compliance (ISO/SOC как продукт), **не** AWS S3 Inventory + Athena и **не** WORM-журнал самого activity trail.

## Предусловия

- Роль администратора в консоли (или Admin API JWT / API token с admin)
- Контрольный бакет с Object Lock / retention (см. [immutable backup](immutable-backup.md))
- Опционально: `STORAGE_ACTIVITY_RETENTION_DAYS` (по умолчанию **90**; `0` — без GC старых записей activity)

## Чеклист (один рабочий день)

1. **Включите Lock** на контрольном бакете (консоль → бакет → Настройки, или Admin `PUT /api/v1/settings/buckets/{name}`).
2. **Загрузите** тестовый объект; при необходимости задайте retention объекта через Admin API.
3. **Попробуйте удалить** — ожидайте HTTP 403 и строку Activity `object_delete_blocked` (Admin JSON delete **и** S3 SigV4 `DeleteObject`).
4. **Состав хранения (CSV)** — Настройки бакета → *Состав хранения*, или:
   ```http
   POST /api/v1/inventory/jobs
   {"bucket":"backups","prefix":"prod/","format":"csv"}
   ```
   Затем `GET /api/v1/inventory/jobs/{id}/download`. Опционально `dest_bucket` / `dest_key` кладут тот же CSV объектом. Cron/schedule в этом релизе — **501** (только manual).
5. **Экспорт Activity** — Администрирование → Activity → Export CSV/JSON (те же фильтры, что в UI), или:
   ```http
   GET /api/v1/activity/export?format=csv&period=30d&bucket=backups
   ```
   Сам экспорт пишется как `activity_exported`.
6. **Папка для аудитора:** inventory CSV + activity CSV/JSON + скрин/JSON настроек Lock.

### Скрипт-помощник (Windows PowerShell)

```powershell
# Host PowerShell — на работающем storage-server
$BaseUrl = "http://127.0.0.1:9000"
$tok = (Invoke-RestMethod -Method POST "$BaseUrl/api/v1/admin/login" `
  -ContentType "application/json" -Body '{"username":"admin","password":"admin"}').token
.\scripts\collect-evidence-pack.ps1 -BaseUrl $BaseUrl -Token $tok -Bucket backups -Prefix "prod/" -Period 30d
```

Создаёт `evidence-pack-<timestamp>/` и `.zip` (inventory + activity + settings бакета + README).

## Колонки CSV (inventory)

`bucket`, `key`, `size`, `last_modified`, `storage_class`, `version_id`, `object_lock_enabled`, `bucket_retention_days`, `retention_mode`, `retention_until`, `legal_hold`

Лимит: **100 000** объектов на job (`truncated: true`, если больше). Jobs в памяти до рестарта процесса; копии в dest-бакете остаются на диске.

## Хранение журнала activity

Раз в час (и один раз при старте процесса) GC удаляет записи старше `STORAGE_ACTIVITY_RETENTION_DAYS` (Bolt и Postgres). По умолчанию 90 дней. Это гигиена диска, **не** immutable audit storage — для долгого хранения выгружайте экспорт в SIEM.

## Связанные материалы

- [Immutable backup](immutable-backup.md)
- [Аудит / Activity](../../administrator-guide/ru/audit.md)
- ADR-0004 Inventory jobs
