# Документация DataSafeS3

**[Русский](ru/README.md)** · **[English](en/README.md)** · **[README репозитория](../README.md)**

**Current Community release:** [v1.7.0](../CHANGELOG.md#170---2026-10-19) (2026-10-19)

Это указатель продукта. Интерфейс консоли доступен на русском и английском. Термины: [глоссарий](ru/glossary.md).

---

## Документы для реестра российского ПО

| Документ | Файл |
|----------|------|
| Руководство пользователя | [ru/user-guide.md](ru/user-guide.md) |
| Инструкция по установке | [ru/installation.md](ru/installation.md) |
| Жизненный цикл ПО | [ru/lifecycle.md](ru/lifecycle.md) |
| Функциональные характеристики | [ru/functional-description.md](ru/functional-description.md) |
| Лицензионное соглашение | [ru/license.md](ru/license.md) |
| Политика конфиденциальности | [ru/privacy-policy.md](ru/privacy-policy.md) |
| Политика безопасности | [ru/security-policy.md](ru/security-policy.md) |
| Порядок поддержки | [ru/support.md](ru/support.md) |

---

## Понять продукт

| Тема | Русский | English |
|------|---------|---------|
| Что это | [getting-started/ru/what-is-datasafe.md](getting-started/ru/what-is-datasafe.md) | [getting-started/en/what-is-datasafe.md](getting-started/en/what-is-datasafe.md) |
| Зачем | [ru/why-datasafe.md](ru/why-datasafe.md) | [en/why-datasafe.md](en/why-datasafe.md) |
| Концептуальная архитектура | [learn/ru/conceptual-architecture.md](learn/ru/conceptual-architecture.md) | [learn/en/conceptual-architecture.md](learn/en/conceptual-architecture.md) |
| Логическая архитектура | [ru/context/architecture.md](ru/context/architecture.md) | [en/context/architecture.md](en/context/architecture.md) |
| Модель данных | [ru/database.md](ru/database.md) | [en/database.md](en/database.md) |
| Онбординг | [getting-started/ru/onboarding.md](getting-started/ru/onboarding.md) | [getting-started/en/onboarding.md](getting-started/en/onboarding.md) |
| Сценарии | [use-cases/README.md](use-cases/README.md) | тот же хаб, страница сценария на EN и RU |
| Совместимость S3 | [ru/compatibility.md](ru/compatibility.md) | [compatibility.md](compatibility.md) |

---

## Развернуть

| Тема | Русский | English |
|------|---------|---------|
| Инструкция по установке | [ru/installation.md](ru/installation.md) | — |
| Первый запуск (Compose) | [getting-started/ru/first-run.md](getting-started/ru/first-run.md) | [getting-started/en/first-run.md](getting-started/en/first-run.md) |
| Интерактивный установщик | [getting-started/ru/installer.md](getting-started/ru/installer.md) | [getting-started/en/installer.md](getting-started/en/installer.md) |
| Оверлеи Compose | [ru/deploy-compose.md](ru/deploy-compose.md) | [../deploy/compose/README.md](../deploy/compose/README.md) |
| Production Compose | [ru/deploy-production.md](ru/deploy-production.md) | [../deploy/compose/production/README.md](../deploy/compose/production/README.md) |
| Мастер настройки | [getting-started/ru/setup-wizard.md](getting-started/ru/setup-wizard.md) | [getting-started/en/setup-wizard.md](getting-started/en/setup-wizard.md) |
| Внешний S3 | [getting-started/ru/s3-configuration.md](getting-started/ru/s3-configuration.md) | [getting-started/en/s3-configuration.md](getting-started/en/s3-configuration.md) |
| Kubernetes, текущий чарт | [ru/kubernetes.md](ru/kubernetes.md) | [kubernetes.md](kubernetes.md) |
| Чарт `datasafes3` | [ru/helm-datasafes3.md](ru/helm-datasafes3.md) | [../deploy/helm/datasafes3/README.md](../deploy/helm/datasafes3/README.md) |
| Прежний чарт `datasafe` | [ru/helm-datasafe.md](ru/helm-datasafe.md) | [../deploy/helm/datasafe/README.md](../deploy/helm/datasafe/README.md) |
| Пакеты .deb / .rpm | [ru/linux-packages.md](ru/linux-packages.md) | [linux-packages.md](linux-packages.md) |
| Зеркало GitVerse | [ru/gitverse-mirror.md](ru/gitverse-mirror.md) | тот же файл |
| Чеклист production | [ru/production-checklist.md](ru/production-checklist.md) | [production-checklist.md](production-checklist.md) |
| Локальная разработка | [ru/context/local-dev.md](ru/context/local-dev.md) | [en/context/local-dev.md](en/context/local-dev.md) |

Kafka и Vault в установку по умолчанию не входят.

---

## Настроить и управлять

| Тема | Русский | English |
|------|---------|---------|
| Руководство администратора | [administrator-guide/ru/README.md](administrator-guide/ru/README.md) | [administrator-guide/en/README.md](administrator-guide/en/README.md) |
| Пользователи | [administrator-guide/ru/users.md](administrator-guide/ru/users.md) | [administrator-guide/en/users.md](administrator-guide/en/users.md) |
| Группы и роли | [administrator-guide/ru/groups-roles.md](administrator-guide/ru/groups-roles.md) | [administrator-guide/en/groups-roles.md](administrator-guide/en/groups-roles.md) |
| Арендаторы | [administrator-guide/ru/tenants.md](administrator-guide/ru/tenants.md) | [administrator-guide/en/tenants.md](administrator-guide/en/tenants.md) |
| Квоты | [administrator-guide/ru/quotas.md](administrator-guide/ru/quotas.md) | [administrator-guide/en/quotas.md](administrator-guide/en/quotas.md) |
| Жизненный цикл объектов | [administrator-guide/ru/lifecycle.md](administrator-guide/ru/lifecycle.md) | [administrator-guide/en/lifecycle.md](administrator-guide/en/lifecycle.md) |
| Репликация Gateway | [administrator-guide/ru/replication.md](administrator-guide/ru/replication.md) | [administrator-guide/en/replication.md](administrator-guide/en/replication.md) |
| Аудит | [administrator-guide/ru/audit.md](administrator-guide/ru/audit.md) | [administrator-guide/en/audit.md](administrator-guide/en/audit.md) |
| LDAP | [administrator-guide/ru/ldap.md](administrator-guide/ru/ldap.md) | [administrator-guide/en/ldap.md](administrator-guide/en/ldap.md) |
| OIDC | [administrator-guide/ru/oidc.md](administrator-guide/ru/oidc.md) | [administrator-guide/en/oidc.md](administrator-guide/en/oidc.md) |
| MFA | [administrator-guide/ru/mfa.md](administrator-guide/ru/mfa.md) | [administrator-guide/en/mfa.md](administrator-guide/en/mfa.md) |
| Мониторинг | [administrator-guide/ru/monitoring.md](administrator-guide/ru/monitoring.md) | [administrator-guide/en/monitoring.md](administrator-guide/en/monitoring.md) |
| Консоль, подробные главы | [ru/user-guide/README.md](ru/user-guide/README.md) | [en/user-guide/README.md](en/user-guide/README.md) |
| Стенд LDAP и Keycloak | [ru/integrations/ldap-keycloak-standalone.md](ru/integrations/ldap-keycloak-standalone.md) | [en/integrations/ldap-keycloak-standalone.md](en/integrations/ldap-keycloak-standalone.md) |

---

## Эксплуатировать

| Тема | Русский | English |
|------|---------|---------|
| Хаб эксплуатации | [operations-guide/ru/README.md](operations-guide/ru/README.md) | [operations-guide/en/README.md](operations-guide/en/README.md) |
| Резервное копирование | [operations-guide/ru/backup-restore.md](operations-guide/ru/backup-restore.md) | [operations-guide/en/backup-restore.md](operations-guide/en/backup-restore.md) |
| Перенос с MinIO | [ru/migrate-from-minio.md](ru/migrate-from-minio.md) | [migrate-from-minio.md](migrate-from-minio.md) |
| Обновление | [operations-guide/ru/upgrade.md](operations-guide/ru/upgrade.md) | [operations-guide/en/upgrade.md](operations-guide/en/upgrade.md) |
| Масштабирование | [operations-guide/ru/scaling.md](operations-guide/ru/scaling.md) | [operations-guide/en/scaling.md](operations-guide/en/scaling.md) |
| Высокая доступность | [operations-guide/ru/high-availability.md](operations-guide/ru/high-availability.md) | [operations-guide/en/high-availability.md](operations-guide/en/high-availability.md) |
| Эталон двух узлов | [operations-guide/ru/reference-deployment-2node.md](operations-guide/ru/reference-deployment-2node.md) | [operations-guide/en/reference-deployment-2node.md](operations-guide/en/reference-deployment-2node.md) |
| Аварийное восстановление | [operations-guide/ru/disaster-recovery.md](operations-guide/ru/disaster-recovery.md) | [operations-guide/en/disaster-recovery.md](operations-guide/en/disaster-recovery.md) |
| События и вебхуки | [ru/integrations/events.md](ru/integrations/events.md) | [en/integrations/events.md](en/integrations/events.md) |
| Kafka | [ru/integrations/kafka.md](ru/integrations/kafka.md) | [en/integrations/kafka.md](en/integrations/kafka.md) |
| Мониторинг | [operations-guide/ru/monitoring.md](operations-guide/ru/monitoring.md) | [operations-guide/en/monitoring.md](operations-guide/en/monitoring.md) |
| Неисправности | [operations-guide/ru/troubleshooting.md](operations-guide/ru/troubleshooting.md) | [operations-guide/en/troubleshooting.md](operations-guide/en/troubleshooting.md) |
| Самооценка безопасности | [operations-guide/ru/security-self-assessment.md](operations-guide/ru/security-self-assessment.md) | [operations-guide/en/security-self-assessment.md](operations-guide/en/security-self-assessment.md) |
| Шифрование объектов | [operations-guide/ru/encryption.md](operations-guide/ru/encryption.md) | [operations-guide/en/encryption.md](operations-guide/en/encryption.md) |
| Шифрование полей метаданных | [operations-guide/ru/field-encryption.md](operations-guide/ru/field-encryption.md) | [operations-guide/en/field-encryption.md](operations-guide/en/field-encryption.md) |
| Vault: подстановка секретов | [operations-guide/ru/secrets-vault.md](operations-guide/ru/secrets-vault.md) | [operations-guide/en/secrets-vault.md](operations-guide/en/secrets-vault.md) |
| Vault Transit | [operations-guide/ru/kms-vault.md](operations-guide/ru/kms-vault.md) | [operations-guide/en/kms-vault.md](operations-guide/en/kms-vault.md) |
| Лаборатория Vault | [ru/vault.md](ru/vault.md) | [../deploy/vault/README.md](../deploy/vault/README.md) |

---

## Справочник и API

| Тема | Русский | English |
|------|---------|---------|
| Шлюз, технически | [ru/context/gateway.md](ru/context/gateway.md) | [en/context/gateway.md](en/context/gateway.md) |
| Дорожная карта | [ru/context/roadmap.md](ru/context/roadmap.md) | [en/context/roadmap.md](en/context/roadmap.md) |
| Статус | [ru/context/project-status.md](ru/context/project-status.md) | [en/context/project-status.md](en/context/project-status.md) |
| ADR | [ru/architecture/adr/README.md](ru/architecture/adr/README.md) | [architecture/adr/README.md](architecture/adr/README.md) |
| Руководство по API | [api-guide/ru/README.md](api-guide/ru/README.md) | [api-guide/en/README.md](api-guide/en/README.md) |
| Аутентификация API | [api-guide/ru/authentication.md](api-guide/ru/authentication.md) | [api-guide/en/authentication.md](api-guide/en/authentication.md) |
| Примеры curl | [api-guide/ru/curl-examples.md](api-guide/ru/curl-examples.md) | [api-guide/en/curl-examples.md](api-guide/en/curl-examples.md) |
| OpenAPI | [api/README.md](api/README.md) | [ru/api/README.md](ru/api/README.md) |
| Swagger | [ru/api/swagger.md](ru/api/swagger.md) | [en/api/swagger.md](en/api/swagger.md) |
| Как это выглядит (галерея скриншотов) | [ru/screenshots-gallery.md](ru/screenshots-gallery.md) | [en/screenshots-gallery.md](en/screenshots-gallery.md) |
| Файлы скриншотов | [images/screenshots/](images/screenshots/) | тот же каталог |
| Диаграммы | [ru/diagrams/README.md](ru/diagrams/README.md) | [diagrams/README.md](diagrams/README.md) |
| Community и Enterprise | [ru/enterprise/community-enterprise-lifecycle.md](ru/enterprise/community-enterprise-lifecycle.md) | [en/enterprise/community-enterprise-lifecycle.md](en/enterprise/community-enterprise-lifecycle.md) |

Swagger UI при установке через Compose открывается как `http://localhost:8080/api/v1/docs` (Caddy проксирует `/api`). Тот же UI слушает процесс на порту API: `http://127.0.0.1:9000/api/v1/docs`.

Журнал изменений продукта: [../CHANGELOG.md](../CHANGELOG.md). Журнал интеграционного API: [ru/api/CHANGELOG.md](ru/api/CHANGELOG.md).
