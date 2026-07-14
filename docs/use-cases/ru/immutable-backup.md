**[English](../en/immutable-backup.md)** | Русский

# Неизменяемый бэкап (Object Lock + versioning)

## Проблема

Инструменты бэкапа (restic, Kopia, Velero, Veeam-подобные S3-таргеты) нуждаются в зоне, где точки восстановления нельзя тихо стереть ransomware или ошибкой оператора. Только Object Lock или только versioning — неполный путь; нужны оба.

## Решение

Используйте DataSafeS3 как **immutable backup** target:

1. Для production предпочтите метаданные **PostgreSQL** ([первый запуск](../../getting-started/ru/first-run.md)).
2. Создайте отдельный бакет (например `backups-db`).
3. Включите **версионирование** (консоль → настройки бакета → Versioning). **Suspend** — только когда нужно перестать выдавать новые version ID, сохранив историю.
4. Включите **Object Lock (WORM)** и выберите режим:
   - **Governance** — удержание; возможен привилегированный override.
   - **Compliance** — жёсткий режим; досрочное удаление в окне retention запрещено.
5. Задайте срок хранения по умолчанию.
6. Выдайте least-privilege S3-ключи для job бэкапа.
7. Направьте restic / Kopia / Velero на S3 endpoint (`:9000` или Caddy) с этими ключами.

Рецепты: [partner cookbook](../../operations-guide/ru/partner-cookbook.md). Восстановление самого DataSafe: [backup & restore](../../operations-guide/ru/backup-restore.md).

## Честные ограничения

- Это **не WORM**, пока Object Lock не включён на бакете.
- Versioning без Lock не заменяет retention/delete-block API.
- Нет обещания 100% parity с AWS Object Lock — проверьте инструмент на lab-бакете.
- После [миграции с MinIO](../../operations-guide/ru/migrate-from-minio.md) заново включите versioning и Object Lock на **цели** — флаги источника не импортируются.

## Проверка

- В консоли: Lock включён, mode задан, versioning Enabled.
- Lab: put → retention → delete отклоняется до истечения срока.
- Smoke: `.\scripts\reference-arch\backup-restore.ps1`

## Архитектура

[ADR-0003](../../architecture/adr/0003-immutable-backup-path.md) (Accepted).
