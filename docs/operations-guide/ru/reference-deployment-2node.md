# Эталонное развёртывание — 2 узла HA + резервное копирование (Community Edition)

**[English](../en/reference-deployment-2node.md)** | Русский

Руководство описывает **поддерживаемый паттерн Community Edition**: active-passive PostgreSQL для метаданных, опциональный read-only `storage-server` standby и внешний backup. **Лицензионных ограничений для HA нет.**

## Топология

```text
[Клиент] → Caddy :8080 → storage-server (primary, запись)
                      ↘ storage-server-standby (STORAGE_READ_ONLY=true, :9001)
PostgreSQL primary ──streaming replication──► PostgreSQL standby
```

## Compose (лаборатория)

```bash
docker compose -f docker-compose.yml -f docker-compose.ha.yml \
  --profile postgres --profile ha-standby up -d --build
```

| Сервис | Роль |
|--------|------|
| `postgres` | Primary метаданных |
| `postgres-standby` | Реплика (`--profile ha-postgres`) |
| `storage-server` | Primary API |
| `storage-server-standby` | DR read (`STORAGE_READ_ONLY=true`) |

На primary задайте `STORAGE_POSTGRES_READ_REPLICA_DSN` для маршрутизации list-запросов на standby.

## Failover (ручной)

1. `scripts/postgres-failover.ps1` или `.sh` (promote, health wait).
2. Обновите `STORAGE_POSTGRES_DSN` на новый primary.
3. Ежеквартально: `scripts/dr-drill.ps1`.

## Kubernetes (Helm)

```bash
helm upgrade --install datasafe ./deploy/helm/datasafe \
  -f deploy/helm/datasafe/values-ha.yaml
```

## Резервное копирование

- **Метаданные:** `pg_dump` с primary или standby.
- **Объекты:** снимок `STORAGE_DATA_DIR` или Gateway replication во внешний S3.

## Проверка

```bash
curl -s http://localhost:8080/healthz | jq .
powershell -File scripts/dr-drill.ps1
```
