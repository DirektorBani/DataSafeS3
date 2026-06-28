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
| Сканирование | govulncheck в CI | ci.yml |
| Раскрытие уязвимостей | SECURITY.md | корень репозитория |

## HA / DR (Community — полный доступ)

Скрипты failover, read-only standby, алерт lag в Grafana — без лицензионных gate.

## Остаточные риски

Ручной failover, erasure MVP лабораторного масштаба; STS — scoped session tokens, привязанные к пользователю (не полный IAM federation).
