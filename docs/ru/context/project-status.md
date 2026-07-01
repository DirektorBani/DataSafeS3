**[English](../../en/context/project-status.md)** | Русский

# Статус проекта

**Обновлено:** 2026-06-30 · **Текущий релиз:** [v1.0.3](https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.3)

## Кратко

**Community Edition v1.0.3** — текущий релиз: S3 API, веб-консоль (EN/RU/DE/FR), метаданные PostgreSQL/Bolt, LDAP/OIDC/MFA/WebAuthn, Object Lock (WORM), Gateway, federation MVP, HA tooling, артефакты поставки (GHCR, SBOM, cosign).

**v1.0.3** добавляет **opt-in шифрование полей метаданных** (CE, без license gate), опциональный **паттерн Vault Agent → env**, усиление CI/Postgres регрессий и вкладку **Security** в админских настройках. По умолчанию поведение как в v1.0.2, пока не включено field encryption.

Patch **v1.0.2** (security): SSRF outbound policy, OIDC `exchange_code`, rate limit login, API `security-status` для слабых секретов.

## Зрелость функций (CE)

| Область | Статус | Примечание |
|---------|--------|------------|
| S3 API (SigV4, multipart, versioning, presign) | **Поставлено** | Порт 9000 |
| Веб-консоль + Admin JSON API | **Поставлено** | Caddy :8080 |
| PostgreSQL + read replica | **Поставлено** | Compose `--profile postgres` |
| LDAP / OIDC SSO | **Поставлено** | OIDC exchange (v1.0.2+); предупреждение о недоступном issuer (AUD-09) |
| MFA / WebAuthn | **Поставлено** | TOTP + passkeys |
| Object Lock (WORM) | **Поставлено** | XML API + консоль |
| Gateway replication | **Поставлено** | Внешний S3 |
| Federation | **Частично (MVP)** | GetObject + ListObjectsV2 proxy |
| HA (failover Postgres, read-only standby) | **Частично** | Ручной promote; Helm `values-ha.yaml` |
| Erasure coding | **Lab MVP** | Не production multi-AZ |
| Supply chain (SBOM + cosign) | **Поставлено** | Оба образа на тегах релиза (v1.0.1+) |
| OpenAPI 3.1 + Swagger UI | **Поставлено** | Community Integration API |
| File collaboration (фазы 1–3) | **Поставлено** | Home bucket, grants, share links, desktop sync |
| Security hardening (v1.0.2+) | **Поставлено** | SSRF policy, rate limits, security-status API |
| Шифрование полей метаданных (v1.0.3) | **Поставлено (opt-in)** | `STORAGE_FIELD_ENCRYPTION_*`, миграция `012` — [field-encryption.md](../operations-guide/ru/field-encryption.md) |
| Vault injection (v1.0.3) | **Поставлено (ops)** | Agent sidecar → `STORAGE_*` env — [secrets-vault.md](../operations-guide/ru/secrets-vault.md) |

## Тестовые гейты

| Гейт | Результат | Когда |
|------|-----------|-------|
| `go test ./...` | PASS | Кампания v1.0.3, 2026-06-30 |
| Feature-audit | PASS | Регрессия 2026-06-30 |
| Playwright e2e-smoke | PASS | CI `smoke.spec.ts`, профиль postgres |
| Postgres FK integration | PASS | `TestNullableFK_team_id` + `TEST_POSTGRES_DSN` |

## Документация

- Двуязычные гайды в `docs/`; upgrade v1.0.3 (EN/RU), field encryption, Vault, CHANGELOG — 2026-06-30.
- Roadmap: [roadmap.md](./roadmap.md).
- Архитектура: [architecture.md](./architecture.md).

## Вне scope CE (план 1.1.0+)

Удаление escape hatch `STORAGE_OUTBOUND_HTTP_ALLOW` (v1.1.0), mobile (Flutter/PWA), Kafka sink, авто-failover orchestrator, production erasure, Vault Transit in-process (Enterprise phase 2).

---

[Индекс документации](../README.md) · [Roadmap](./roadmap.md) · [CHANGELOG](../../../CHANGELOG.md)
