# Эталонное развёртывание — HA v2 (Community Edition)

**[English](../en/reference-deployment-2node.md)** | Русский

HA v2 в CE: **erasure для объектов**, **оркестрируемый failover метаданных Postgres**, опциональная **site replication** на второй DataSafeS3. Лицензионных ограничений нет.

## Топология (рекомендуемая)

```text
[Клиент] → Caddy → storage-server (leader, запись)
                    │
                    ├─ STORAGE_OBJECT_BACKEND=erasure (4+2 или dev 2+1)
                    ├─ Postgres primary ──streaming──► standby + ha_leader_lock
                    └─ site replication (async) ──► Site B
```

## Legacy standby (deprecated)

Общий FS + `storage-server-standby` (read-only) — только для DR drill. Новые установки: erasure + metadata failover. См. [scaling.md](../ru/scaling.md).

## Compose (лаборатория Windows)

| Профиль | Назначение | Скрипт |
|---------|------------|--------|
| HA lab | Postgres + 3 storage | `scripts\ha\start-ha-stack.ps1` |
| Erasure | 6 shard volumes | `deploy/compose/docker-compose.ha-erasure.yml`, `scripts\ha\test-erasure-backend.ps1` |
| Site replication | Site A → Site B | `scripts\ha\start-site-replication-lab.ps1`, `scripts\ha\test-site-replication.ps1` |

## Failover метаданных

1. Остановить leader или `STORAGE_READ_ONLY=true`.
2. `scripts\ha\failover-metadata.ps1`.
3. Обновить `STORAGE_POSTGRES_DSN`, перезапустить storage-server.
4. Проверить `/healthz`: `is_leader=true`.

## Проверка

```powershell
scripts\ha\test-ha-cluster.ps1
scripts\ha\test-erasure-backend.ps1
scripts\ha\test-site-replication.ps1
```

Подробнее: [backup-restore](../ru/backup-restore.md), [disaster-recovery](../ru/disaster-recovery.md).
