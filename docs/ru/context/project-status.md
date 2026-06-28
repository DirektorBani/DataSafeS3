**[English](../../en/context/project-status.md)** | Русский

# Статус проекта

**Обновлено:** 2026-06-28 · **Текущий релиз:** v1.0.1 (тег)

## Кратко

\*\*Community Edition v1.0.1\*\* — текущий публичный релиз DataSafeS3 (patch после v1.0.0): S3 API, веб-консоль (EN/RU/DE/FR), метаданные PostgreSQL/Bolt, LDAP/OIDC/MFA/WebAuthn, Object Lock (WORM), Gateway, federation MVP, HA tooling, артефакты поставки (GHCR, SBOM, cosign).

Patch **v1.0.1** — supply chain (SBOM console, артефакты GitHub Release), синхронизация документации, AUD-08 UX ошибок списка бакетов; **без** новых продуктовых capability.

## Зрелость функций (CE)

| Область | Статус | Примечание |
|---------|--------|------------|
| S3 API (SigV4, multipart, versioning, presign) | **Поставлено** | Порт 9000 |
| Веб-консоль + Admin JSON API | **Поставлено** | Caddy :8080 |
| PostgreSQL + read replica | **Поставлено** | Compose `--profile postgres` |
| LDAP / OIDC SSO | **Поставлено** | Недоступный issuer → предупреждение в консоли (AUD-09) |
| MFA / WebAuthn | **Поставлено** | TOTP + passkeys |
| Object Lock (WORM) | **Поставлено** | XML API + консоль |
| Gateway replication | **Поставлено** | Внешний S3 |
| Federation | **Частично (MVP)** | GetObject + ListObjectsV2 proxy |
| HA (failover Postgres, read-only standby) | **Частично** | Ручной promote; Helm `values-ha.yaml` |
| Erasure coding | **Lab MVP** | Не production multi-AZ |
| Supply chain (SBOM + cosign) | **Поставлено** | Оба образа на тегах релиза (v1.0.1+) |
| OpenAPI 3.1 + Swagger UI | **Поставлено** | Community Integration API |
| File collaboration (фазы 1–3) | **Поставлено** | Home bucket, grants, share links |

## Тестовые гейты (последняя проверка)

| Гейт | Результат | Когда |
|------|-----------|-------|
| `go test ./...` | PASS | Кампания v1.0.1, 2026-06-28 |
| Feature-audit | 101 PASS / 2 SKIP | Регрессия 2026-06-28 |
| Playwright E2E | 6/6 PASS | 2026-06-28 (6 specs) |

## Документация

- Двуязычные гайды: 35 EN = 35 RU в `docs/`.
- Audit roadmap: [roadmap.md](./roadmap.md) — AUD-08/09 закрыты в scope v1.0.1.
- Архитектура: [architecture.md](./architecture.md) · [дорожная карта](../../specs/roadmap/README.md).

## Вне scope CE (план 1.1.0+)

Mobile (Flutter/PWA), Kafka, автоматический failover orchestrator, production erasure, полная de/fr документация, publish образов на каждый push в `main`.

---

[Индекс документации](../README.md) · [Roadmap](./roadmap.md) · [Архитектура](./architecture.md)
