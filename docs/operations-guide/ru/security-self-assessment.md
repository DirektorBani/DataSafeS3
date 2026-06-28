# Лёгкая самооценка безопасности

**[English](../en/security-self-assessment.md)** | Русский

Внутреннее резюме для enterprise-ревью — **не** сертификат стороннего pentest.

## Реализованные меры

| Область | Статус | Доказательство |
|---------|--------|----------------|
| Аутентификация | JWT, SigV4, WebAuthn/TOTP | feature-audit B6–B10 |
| Авторизация | RBAC, policies, tenants | feature-audit C12–C16 |
| Аудит | Activity log, share events | Admin → Activity |
| Supply chain | SBOM + Cosign на тегах (оба образа) | release workflow, [SECURITY.md](../../../SECURITY.md) |
| Секреты | Env / K8s, `STORAGE_STRICT_SECRETS`, security-status API | Helm `values-production.yaml` |
| SSRF / исходящие URL | urlpolicy для sinks, hooks, notifications | `STORAGE_DEV`, `STORAGE_OUTBOUND_HTTP_ALLOW` |
| OIDC сессия | Exchange code (без JWT в URL браузера) | `POST /auth/oidc/exchange` |
| Rate limiting | Login по IP | `STORAGE_RATE_LIMIT_LOGIN` |
| Сканирование | govulncheck в CI | ci.yml |
| Раскрытие уязвимостей | SECURITY.md | корень репозитория |

## HA / DR (Community — полный доступ)

Скрипты failover, read-only standby, алерт lag в Grafana — без лицензионных gate.

## Остаточные риски

Ручной failover, erasure MVP лабораторного масштаба; STS — scoped session tokens, привязанные к пользователю (не полный IAM federation).
