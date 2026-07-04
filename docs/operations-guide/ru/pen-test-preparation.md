**[English](../en/pen-test-preparation.md)** | Русский

# Подготовка к penetration test (внутренний чеклист)

**Не сертификат.** Для привлечения стороннего assessor или внутреннего red-team против DataSafeS3 CE.

## Документ scope

| Пункт | Включить |
|-------|----------|
| **Версия** | Тег релиза (например v1.1.0) + digest образа console |
| **Поверхность** | `storage-server` :9000, console :8080, опционально Postgres |
| **Вне scope** | Keycloak/LDAP test containers, если не оговорено |
| **Правила** | Без production data; отдельный lab / compose |

## Базовая среда

1. Деплой по [upgrade](upgrade.md) с **ротированными секретами**.
2. `STORAGE_METRICS_TOKEN` — `/metrics` без bearer → 401.
3. `STORAGE_OUTBOUND_HTTP_ALLOW` **не задавать** (удалена в v1.1.0).
4. `GET /api/v1/settings/security-status` — пустой `weak_secrets`.
5. Приложить [security self-assessment](security-self-assessment.md) и feature-audit summary.

## Приоритетные сценарии

Auth, SSRF, IDOR/tenant RBAC, S3 key traversal, metrics disclosure, cosign/SBOM.

## После оценки

Триаж в GitHub Security Advisories; обновление roadmap AUD-*; публичное раскрытие только после coordinated disclosure.

См. [SECURITY.md](../../../SECURITY.md), [TZ v1.1.0](../../specs/v1.1.0-trust-debt-oss-growth-tz.md).
