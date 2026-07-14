**[English](../../en/context/project-status.md)** | Русский

# Статус проекта

**Обновлено:** 2026-07-14 · **Текущий релиз:** [v1.2.0](https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.2.0)

## Кратко

**Community Edition v1.2.0** — текущий релиз: **immutable backup golden path** (`retention_mode` Object Lock + Suspend versioning в консоли, use-case EN/RU), AUD-16 `object_count` при удалении папки, плюс MinIO migration kit из v1.1.1. Платформа: S3 API, веб-консоль (EN/RU/DE/FR), метаданные PostgreSQL/Bolt, LDAP/OIDC/MFA/WebAuthn, Object Lock (WORM), Gateway, federation MVP, Teams, защищённые `/metrics`, hash-only share tokens и HA v2 lab tooling.

**v1.1.0** закрывает trust-debt C01-C21 и поставляет **HA v2 CE lab foundation**: erasure backend, leader lock в Postgres, site replication и локальные скрипты проверки. Это scope **lab / not production multi-AZ**: без автоматического failover orchestrator, без production Patroni-кластера и без обещания petabyte 4+2 multi-host.

Patch **v1.0.3**: opt-in field encryption, паттерн Vault Agent → env, усиление CI/Postgres регрессий и вкладка **Security** в админских настройках.

## Зрелость функций (CE)

| Область | Статус | Примечание |
|---------|--------|------------|
| S3 API (SigV4, multipart, versioning, presign) | **Поставлено** | Порт 9000 |
| Веб-консоль + Admin JSON API | **Поставлено** | Caddy :8080 |
| PostgreSQL + read replica | **Поставлено** | Compose `--profile postgres` |
| LDAP / OIDC SSO | **Поставлено** | OIDC exchange (v1.0.2+); предупреждение о недоступном issuer (AUD-09) |
| MFA / WebAuthn | **Поставлено** | TOTP + passkeys |
| Object Lock (WORM) | **Поставлено** | XML API + консоль; `retention_mode` GOVERNANCE/COMPLIANCE (v1.2.0) |
| Gateway replication | **Поставлено** | Внешний S3 |
| Federation | **Частично (MVP)** | GetObject + ListObjectsV2 proxy |
| Teams admin API + консоль | **Поставлено** | Admin → Teams, назначение пользователей, OpenAPI full spec |
| Bearer token для metrics | **Поставлено** | `STORAGE_METRICS_TOKEN` защищает `/metrics`, если задан |
| HA v2 lab foundation | **Поставлено (lab)** | Erasure, leader lock, site replication; не production multi-AZ |
| HA (failover Postgres, read-only standby) | **Частично** | Ручной promote; Helm `values-ha.yaml` |
| Erasure coding | **Lab foundation** | `STORAGE_OBJECT_BACKEND=erasure`; production multi-AZ — future hardening |
| Supply chain (SBOM + cosign) | **Поставлено** | Оба образа на тегах релиза (v1.0.1+) |
| OpenAPI 3.1 + Swagger UI | **Поставлено** | Community Integration API |
| File collaboration (фазы 1–3) | **Поставлено** | Home bucket, grants, share links, desktop sync |
| Security hardening (v1.0.2+) | **Поставлено** | SSRF policy, rate limits, security-status API |
| Шифрование полей метаданных (v1.0.3) | **Поставлено (opt-in)** | `STORAGE_FIELD_ENCRYPTION_*`, миграция `012` — [field-encryption.md](../operations-guide/ru/field-encryption.md) |
| Vault injection (v1.0.3) | **Поставлено (ops)** | Agent sidecar → `STORAGE_*` env — [secrets-vault.md](../operations-guide/ru/secrets-vault.md) |

## Тестовые гейты

| Гейт | Результат | Когда |
|------|-----------|-------|
| `go test ./...` | PASS | Release prep 2026-07-04 |
| Feature-audit | PASS | 112 PASS / 0 FAIL / 1 SKIP, 2026-07-04 |
| Playwright 7 specs | PASS | 9 tests across 7 spec files, 2026-07-04 |
| HA lab Option B | PASS | `run-all-ha-tests.ps1 -FreshVolumes -SkipBuild`, 0 FAIL |

## Документация

- Двуязычные гайды в `docs/`; upgrade v1.1.0 (EN/RU), HA lab docs, pen-test prep, CHANGELOG — 2026-07-04.
- Roadmap: [roadmap.md](./roadmap.md).
- Архитектура: [architecture.md](./architecture.md).

## Вне scope CE (future)

Mobile (Flutter/PWA), Kafka sink, авто-failover orchestrator, production multi-AZ erasure, Vault Transit in-process (Enterprise phase 2).

## v1.1.0 shipped scope

Charter: trust-debt + OSS growth — внутреннее ТЗ в `D:\datasafe_tz\specs\v1.1.0\` (см. [docs/specs/README.md](../specs/README.md)).

| Пункт | Статус |
|-------|--------|
| Удаление `STORAGE_OUTBOUND_HTTP_ALLOW` | **Поставлено** |
| `STORAGE_METRICS_TOKEN` на `/metrics` | **Поставлено** |
| Upgrade EN/RU + CHANGELOG | **Поставлено** |
| Playwright CI ≥6 specs (P1) | **Готово** |
| Teams admin REST + UI (P1) | **Готово** |
| OIDC Keycloak E2E nightly (P1) | **Готово** |
| MFA wizard AUD-17 (P1) | **Готово** |
| Feature-audit AUD-15/18 (P1) | **Готово** |
| CONTRIBUTING + ref-arch (P1) | **Готово** |
| Pen-test prep doc (P1) | **Готово** |
| Grafana panel smoke AUD-10 (P2) | **Готово** |
| SDK examples + GHCR main publish (P2) | **Готово** |
| de/fr getting-started stubs (P2) | **Готово** |
| Share link hash-only C21 (P2) | **Готово** |
| OpenAPI teams routes (C06-7) | **Готово** |
| HA v2 CE lab foundation | **Поставлено (lab)** |

---

[Индекс документации](../README.md) · [Roadmap](./roadmap.md) · [CHANGELOG](../../../CHANGELOG.md)
