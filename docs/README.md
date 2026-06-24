# DataSafeS3 Documentation / Документация DataSafeS3

**[English](en/README.md)** · **[Русский](ru/README.md)** · **[Repository README](../README.md)**

Product documentation for DataSafeS3 — a self-hosted storage platform for secure and governed data management. Documentation focuses on **product value**, not comparisons with other products.

---

## Learn — понять продукт

What DataSafeS3 is, how it works, and how data and access are modeled.

| Topic | English | Русский |
|-------|---------|---------|
| **Overview** | [getting-started/en/what-is-datasafe.md](getting-started/en/what-is-datasafe.md) | [getting-started/ru/what-is-datasafe.md](getting-started/ru/what-is-datasafe.md) |
| **Why DataSafeS3** | [en/why-datasafe.md](en/why-datasafe.md) | [ru/why-datasafe.md](ru/why-datasafe.md) |
| **Conceptual architecture** | [learn/en/conceptual-architecture.md](learn/en/conceptual-architecture.md) | [learn/ru/conceptual-architecture.md](learn/ru/conceptual-architecture.md) |
| **Logical architecture (technical)** | [en/context/architecture.md](en/context/architecture.md) | [ru/context/architecture.md](ru/context/architecture.md) |
| **Security model** | [administrator-guide/en/mfa.md](administrator-guide/en/mfa.md) · [ldap](administrator-guide/en/ldap.md) · [oidc](administrator-guide/en/oidc.md) | [ru/mfa](administrator-guide/ru/mfa.md) · [ldap](administrator-guide/ru/ldap.md) · [oidc](administrator-guide/ru/oidc.md) |
| **Data model** | [en/database.md](en/database.md) | [ru/database.md](ru/database.md) |
| **Onboarding checklist** | [getting-started/en/onboarding.md](getting-started/en/onboarding.md) | [getting-started/ru/onboarding.md](getting-started/ru/onboarding.md) |
| **Use cases** | [use-cases/README.md](use-cases/README.md) | same hub (EN/RU per scenario) |

---

## Deploy — развернуть {#deploy}

| Topic | English | Русский |
|-------|---------|---------|
| **First run (Docker Compose)** | [getting-started/en/first-run.md](getting-started/en/first-run.md) | [getting-started/ru/first-run.md](getting-started/ru/first-run.md) |
| **Setup wizard** | [getting-started/en/setup-wizard.md](getting-started/en/setup-wizard.md) | [getting-started/ru/setup-wizard.md](getting-started/ru/setup-wizard.md) |
| **External S3 (optional)** | [getting-started/en/s3-configuration.md](getting-started/en/s3-configuration.md) | [getting-started/ru/s3-configuration.md](getting-started/ru/s3-configuration.md) |
| **Kubernetes / Helm** | [../deploy/helm/datasafe/README.md](../deploy/helm/datasafe/README.md) | same |
| **Production deployment** | [operations-guide/en/README.md](operations-guide/en/README.md) | [operations-guide/ru/README.md](operations-guide/ru/README.md) |
| **Local development** | [en/context/local-dev.md](en/context/local-dev.md) | [ru/context/local-dev.md](ru/context/local-dev.md) |

---

## Configure — настроить

| Topic | English | Русский |
|-------|---------|---------|
| **Storage & buckets** | [getting-started/en/first-bucket.md](getting-started/en/first-bucket.md) | [getting-started/ru/first-bucket.md](getting-started/ru/first-bucket.md) |
| **LDAP** | [administrator-guide/en/ldap.md](administrator-guide/en/ldap.md) | [administrator-guide/ru/ldap.md](administrator-guide/ru/ldap.md) |
| **OIDC / SSO** | [administrator-guide/en/oidc.md](administrator-guide/en/oidc.md) | [administrator-guide/ru/oidc.md](administrator-guide/ru/oidc.md) |
| **MFA** | [administrator-guide/en/mfa.md](administrator-guide/en/mfa.md) | [administrator-guide/ru/mfa.md](administrator-guide/ru/mfa.md) |
| **Monitoring & logging** | [administrator-guide/en/monitoring.md](administrator-guide/en/monitoring.md) | [administrator-guide/ru/monitoring.md](administrator-guide/ru/monitoring.md) |
| **LDAP/SSO test stack** | [en/integrations/ldap-keycloak-standalone.md](en/integrations/ldap-keycloak-standalone.md) | [ru/integrations/ldap-keycloak-standalone.md](ru/integrations/ldap-keycloak-standalone.md) |

Environment variables and system settings: [User guide — Administrator settings](en/user-guide/README.md) · [RU](ru/user-guide/README.md)

---

## Manage — управлять

| Topic | English | Русский |
|-------|---------|---------|
| **Administrator guide hub** | [administrator-guide/en/README.md](administrator-guide/en/README.md) | [administrator-guide/ru/README.md](administrator-guide/ru/README.md) |
| **Users & RBAC** | [administrator-guide/en/users.md](administrator-guide/en/users.md) | [administrator-guide/ru/users.md](administrator-guide/ru/users.md) |
| **Groups & roles** | [administrator-guide/en/groups-roles.md](administrator-guide/en/groups-roles.md) | [administrator-guide/ru/groups-roles.md](administrator-guide/ru/groups-roles.md) |
| **Tenants** | [administrator-guide/en/tenants.md](administrator-guide/en/tenants.md) | [administrator-guide/ru/tenants.md](administrator-guide/ru/tenants.md) |
| **Quotas** | [administrator-guide/en/quotas.md](administrator-guide/en/quotas.md) | [administrator-guide/ru/quotas.md](administrator-guide/ru/quotas.md) |
| **Lifecycle** | [administrator-guide/en/lifecycle.md](administrator-guide/en/lifecycle.md) | [administrator-guide/ru/lifecycle.md](administrator-guide/ru/lifecycle.md) |
| **Replication (Gateway)** | [administrator-guide/en/replication.md](administrator-guide/en/replication.md) | [administrator-guide/ru/replication.md](administrator-guide/ru/replication.md) |
| **Audit** | [administrator-guide/en/audit.md](administrator-guide/en/audit.md) | [administrator-guide/ru/audit.md](administrator-guide/ru/audit.md) |
| **User guide (console)** | [en/user-guide/README.md](en/user-guide/README.md) | [ru/user-guide/README.md](ru/user-guide/README.md) |

---

## Operate — эксплуатировать

| Topic | English | Русский |
|-------|---------|---------|
| **Operations hub** | [operations-guide/en/README.md](operations-guide/en/README.md) | [operations-guide/ru/README.md](operations-guide/ru/README.md) |
| **Backup & restore** | [operations-guide/en/backup-restore.md](operations-guide/en/backup-restore.md) | [operations-guide/ru/backup-restore.md](operations-guide/ru/backup-restore.md) |
| **Upgrade** | [operations-guide/en/upgrade.md](operations-guide/en/upgrade.md) | [operations-guide/ru/upgrade.md](operations-guide/ru/upgrade.md) |
| **Scaling** | [operations-guide/en/scaling.md](operations-guide/en/scaling.md) | [operations-guide/ru/scaling.md](operations-guide/ru/scaling.md) |
| **2-node HA reference** | [operations-guide/en/reference-deployment-2node.md](operations-guide/en/reference-deployment-2node.md) | [operations-guide/ru/reference-deployment-2node.md](operations-guide/ru/reference-deployment-2node.md) |
| **Partner integrations** | [operations-guide/en/partner-cookbook.md](operations-guide/en/partner-cookbook.md) | [operations-guide/ru/partner-cookbook.md](operations-guide/ru/partner-cookbook.md) |
| **Disaster recovery** | [operations-guide/en/disaster-recovery.md](operations-guide/en/disaster-recovery.md) | [operations-guide/ru/disaster-recovery.md](operations-guide/ru/disaster-recovery.md) |
| **Monitoring** | [operations-guide/en/monitoring.md](operations-guide/en/monitoring.md) | [operations-guide/ru/monitoring.md](operations-guide/ru/monitoring.md) |
| **Troubleshooting** | [operations-guide/en/troubleshooting.md](operations-guide/en/troubleshooting.md) | [operations-guide/ru/troubleshooting.md](operations-guide/ru/troubleshooting.md) |
| **Security self-assessment** | [operations-guide/en/security-self-assessment.md](operations-guide/en/security-self-assessment.md) | [operations-guide/ru/security-self-assessment.md](operations-guide/ru/security-self-assessment.md) |
| **Extension hooks (example)** | [../examples/extension-hook/README.md](../examples/extension-hook/README.md) | same |

---

## Reference — справочник

| Topic | English | Русский |
|-------|---------|---------|
| **Database schema** | [en/database.md](en/database.md) | [ru/database.md](ru/database.md) |
| **Gateway (technical)** | [en/context/gateway.md](en/context/gateway.md) | [ru/context/gateway.md](ru/context/gateway.md) |
| **Roadmap** | [en/context/roadmap.md](en/context/roadmap.md) | [ru/context/roadmap.md](ru/context/roadmap.md) |
| **Specifications (TZ)** | [en/specs/](en/specs/) | [ru/specs/](ru/specs/) |
| **File collaboration (status)** | [en/specs/file-collaboration-status.md](en/specs/file-collaboration-status.md) | [ru/specs/file-collaboration-status.md](ru/specs/file-collaboration-status.md) |
| **File collaboration (TZ)** | [en/specs/file-collaboration-tz.md](en/specs/file-collaboration-tz.md) | [ru/specs/file-collaboration-tz.md](ru/specs/file-collaboration-tz.md) |
| **File collaboration Phase 4 backlog** | [en/specs/file-collaboration-phase4-backlog.md](en/specs/file-collaboration-phase4-backlog.md) | [ru/specs/file-collaboration-phase4-backlog.md](ru/specs/file-collaboration-phase4-backlog.md) |
| **UI screenshots** | [images/screenshots/](images/screenshots/) | same |
| **Diagrams (Mermaid)** | [diagrams/README.md](diagrams/README.md) | same |

---

## API

| Topic | English | Русский |
|-------|---------|---------|
| **API guide** | [api-guide/en/README.md](api-guide/en/README.md) | [api-guide/ru/README.md](api-guide/ru/README.md) |
| **Authentication** | [api-guide/en/authentication.md](api-guide/en/authentication.md) | [api-guide/ru/authentication.md](api-guide/ru/authentication.md) |
| **curl examples** | [api-guide/en/curl-examples.md](api-guide/en/curl-examples.md) | [api-guide/ru/curl-examples.md](api-guide/ru/curl-examples.md) |
| **Swagger UI** | [en/api/swagger.md](en/api/swagger.md) | [ru/api/swagger.md](ru/api/swagger.md) |
| **OpenAPI specs** | [api/openapi.yaml](api/openapi.yaml) · [openapi-full.yaml](api/openapi-full.yaml) | same files |

Live Swagger: `http://localhost:8080/api/v1/docs`

---

## Enterprise — Community and commercial edition

How features move between Community (Apache-2.0) and Enterprise, and how requests are evaluated.

| Topic | English | Русский |
|-------|---------|---------|
| **Community ↔ Enterprise lifecycle** | [en/enterprise/community-enterprise-lifecycle.md](en/enterprise/community-enterprise-lifecycle.md) | [ru/enterprise/community-enterprise-lifecycle.md](ru/enterprise/community-enterprise-lifecycle.md) |
| **Feature request evaluation template** | [en/enterprise/feature-request-evaluation.md](en/enterprise/feature-request-evaluation.md) | [ru/enterprise/feature-request-evaluation.md](ru/enterprise/feature-request-evaluation.md) |

---

## Documentation standards

Product positioning and writing rules for contributors: [Product documentation TZ](en/specs/product-documentation-tz.md) · [RU](ru/specs/product-documentation-tz.md)
