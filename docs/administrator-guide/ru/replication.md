**[English](../en/replication.md)** | Русский

# Репликация

Три режима для DataSafeS3↔DataSafeS3 (плюс Gateway для внешнего S3) — **не путать**:

| Функция | Цель | Сценарий |
|---------|------|----------|
| **Gateway replication** | Внешний S3 | Backup, hybrid cloud |
| **Site replication** | Другой **DataSafeS3** (AK/SK) | Lab, legacy |
| **Trusted cluster replication** | **Paired** DataSafeS3 (mTLS) | DR с автоматическим trust — **предпочтительно** |

![Gateway](../../images/screenshots/gateway.png)

## Gateway replication (external S3)

Асинхронная репликация buckets во внешний S3.

```http
GET  /api/v1/gateway/connections
POST /api/v1/gateway/replication
POST /api/v1/gateway/replication/{id}/sync
```

Руководство: [Gateway](../../ru/user-guide/06-gateway-i-minio.md) · [gateway.md](../../ru/context/gateway.md)

---

## Site replication (DataSafeS3 ↔ DataSafeS3)

Async one-way между двумя сайтами (CE, v1.1.0).

### Требования

- На **источнике**: `STORAGE_SITE_REPLICATION_ENABLED=true`
- Peer: S3 endpoint (path-style), access key, secret key
- Исходящий URL: peer должен быть доступен (`STORAGE_DEV=true` для lab HTTP)

### Лаборатория

```powershell
scripts\ha\start-site-replication-lab.ps1 -FreshVolumes
scripts\ha\test-site-replication.ps1
```

### API

```http
GET/POST/DELETE /api/v1/site-replication/peers
GET/POST/DELETE /api/v1/site-replication/rules
POST /api/v1/site-replication/rules/{id}/sync
GET /api/v1/site-replication/status
```

См. [scaling](../../operations-guide/ru/scaling.md) · [disaster-recovery](../../operations-guide/ru/disaster-recovery.md)

---

## Trusted cluster replication (mTLS, v1.1.0)

Репликация на **доверенный remote-кластер** после pairing (`dsjoin_*`). Транспорт — **mTLS**; pairing автоматический.

### Когда использовать

- Два сайта DataSafeS3 в разных сетях
- Нужны hash токенов, авто-rotation cert и revoke без plaintext secrets в БД

### Настройка

1. Pairing — [trusted-clusters.md](trusted-clusters.md)
2. Правило репликации на карточке remote
3. `STORAGE_TRUSTED_CLUSTER_REPL_ENABLED=true`

Подробнее: [Operations — trusted clusters](../../operations-guide/ru/trusted-clusters.md)
