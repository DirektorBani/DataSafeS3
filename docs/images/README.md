# Images / Изображения

## UI screenshots (v1.7.0)

Every screenshot was captured on a real DataSafeS3 v1.7.0 installation from the images published on GitVerse, with all integrations running (LDAP, Keycloak/OIDC, Kafka, NATS, webhooks, Vault, Grafana, Prometheus, a second site and a MinIO source) and fictional demo data. The same files are used by the READMEs, the documentation and the project website, and [`screenshots/manifest.json`](screenshots/manifest.json) lists them with their captions (`id`, files, ru/en caption, SHA-256).

Captioned overview: [What it looks like](../en/screenshots-gallery.md) · [Как это выглядит](../ru/screenshots-gallery.md).

Layout: `screenshots/ru/` (Russian UI), `screenshots/en/` (English UI), `screenshots/common/` (language-neutral: terminal output and external tools). Light-theme variants end with `-light`.

| File | Group | Caption |
|------|-------|---------|
| [common/install-clone.png](screenshots/common/install-clone.png) | Installation | Cloning the repository and running install.ps1: Docker and port checks and profile selection (terminal output, Windows 11 + Docker Desktop). |
| [common/install-identity.png](screenshots/common/install-identity.png) | Installation | The installer starts the test LDAP and Keycloak and prints the lab endpoints and credentials (terminal output). |
| [common/install-compose.png](screenshots/common/install-compose.png) | Installation | docker compose up from the published GitVerse images and the console readiness check (terminal output). |
| [common/compose-ps-core.png](screenshots/common/compose-ps-core.png) | Installation | Base installation: storage server, Caddy, PostgreSQL, Prometheus, Grafana and Kafka are all running (docker compose ps). |
| [common/compose-ps-full.png](screenshots/common/compose-ps-full.png) | Installation | Full installation with every integration: two storage nodes, Kafka and Kafka UI, NATS, Vault, a MinIO source, a second site and a webhook viewer (docker ps). |
| [en/login.png](screenshots/en/login.png) | Console: overview, buckets and objects | Console sign-in page with the language switcher at the top right and the SSO option. |
| [en/dashboard.png](screenshots/en/dashboard.png) | Console: overview, buckets and objects | Overview: buckets, storage used, objects, access keys and API status (dark theme, demo data). |
| [en/buckets.png](screenshots/en/buckets.png) | Console: overview, buckets and objects | Bucket list: demo buckets with different access levels (dark theme). |
| [en/object-browser.png](screenshots/en/object-browser.png) | Console: overview, buckets and objects | Object browser: folders, upload, share links and the metadata panel of the selected object (dark theme). |
| [en/object-versions.png](screenshots/en/object-versions.png) | Console: overview, buckets and objects | Object versions: each version has an ID, size and date; a version can be downloaded or deleted. |
| [en/bucket-lifecycle.png](screenshots/en/bucket-lifecycle.png) | Console: overview, buckets and objects | Bucket lifecycle rules: automatic deletion of objects by prefix after a set number of days. |
| [en/bucket-access.png](screenshots/en/bucket-access.png) | Console: overview, buckets and objects | Bucket settings, Access tab: tenant access and access to folders (prefixes) inside the bucket. |
| [en/bucket-sharing.png](screenshots/en/bucket-sharing.png) | Console: overview, buckets and objects | Bucket settings, File sharing tab: public access, share links and their lifetime. |
| [en/bucket-encryption.png](screenshots/en/bucket-encryption.png) | Console: overview, buckets and objects | Bucket encryption: server key or Vault Transit key (the secure-vault bucket). |
| [en/keys.png](screenshots/en/keys.png) | Access, users and audit | S3 access keys and API tokens; secret keys are not displayed. |
| [en/usage.png](screenshots/en/usage.png) | Access, users and audit | Usage: size and object count per bucket and storage growth. |
| [en/users.png](screenshots/en/users.png) | Access, users and audit | Users: roles, quotas, statuses and last sign-in (fictional users). |
| [en/teams.png](screenshots/en/teams.png) | Access, users and audit | Teams: grouping users for access grants. |
| [en/tenants.png](screenshots/en/tenants.png) | Access, users and audit | Tenants: isolated organisations with their own users and buckets. |
| [en/activity.png](screenshots/en/activity.png) | Access, users and audit | Activity log: sign-ins, administrator actions and delivery events (filters and CSV/JSON export). |
| [en/webhooks.png](screenshots/en/webhooks.png) | Access, users and audit | Webhooks: object event subscriptions (a delivery viewer is attached). |
| [en/profile.png](screenshots/en/profile.png) | Access, users and audit | Profile: two-factor authentication (TOTP), passkeys (WebAuthn) and password change. |
| [en/settings.png](screenshots/en/settings.png) | Settings and integrations | Administrator settings: soft delete, LDAP/AD and OIDC/SSO. |
| [en/settings-buckets.png](screenshots/en/settings-buckets.png) | Settings and integrations | Bucket settings: apply parameters to several buckets at once. |
| [en/settings-security.png](screenshots/en/settings-security.png) | Settings and integrations | Security: public access, pre-signed links, lifetimes and MFA. |
| [en/settings-events.png](screenshots/en/settings-events.png) | Settings and integrations | Event delivery: Kafka, NATS JetStream and webhooks, with queue and delivery counters. |
| [en/settings-kms.png](screenshots/en/settings-kms.png) | Settings and integrations | Vault (KMS): connection to Vault Transit for encryption; the token is not displayed. |
| [en/settings-migrate.png](screenshots/en/settings-migrate.png) | Settings and integrations | Migration from MinIO: the wizard builds the command for the migration utility. |
| [en/gateway.png](screenshots/en/gateway.png) | Gateway, replication and cluster | Gateway: connections to an external S3 storage (the Site B demo site) with a connection test, and the add-connection form. |
| [en/gateway-rules.png](screenshots/en/gateway-rules.png) | Gateway, replication and cluster | Gateway: replication rules and manual sync. |
| [en/gateway-jobs.png](screenshots/en/gateway-jobs.png) | Gateway, replication and cluster | Gateway: sync jobs and their results. |
| [en/gateway-status.png](screenshots/en/gateway-status.png) | Gateway, replication and cluster | Gateway: status — 2/2 connections OK, one rule, empty replication queue, 4 tasks replicated. |
| [en/cluster.png](screenshots/en/cluster.png) | Gateway, replication and cluster | Clusters: the local node and trusted clusters. |
| [common/ha-status.png](screenshots/common/ha-status.png) | Gateway, replication and cluster | Two-node HA: /cluster/status reports HA enabled and a leader elected (terminal output). |
| [common/monitoring.png](screenshots/common/monitoring.png) | Monitoring and external services | Grafana overview dashboard: requests, storage, object count, latency and HTTP errors (Prometheus data). |
| [common/grafana-buckets.png](screenshots/common/grafana-buckets.png) | Monitoring and external services | Grafana buckets dashboard: size and object count per bucket. |
| [common/prometheus-targets.png](screenshots/common/prometheus-targets.png) | Monitoring and external services | Prometheus: the storage-server scrape target is UP. |
| [common/kafka-ui-topics.png](screenshots/common/kafka-ui-topics.png) | Monitoring and external services | Kafka UI: the datasafe.events topic with storage events. |
| [common/kafka-ui-messages.png](screenshots/common/kafka-ui-messages.png) | Monitoring and external services | Kafka UI: messages of the datasafe.events topic: object created and deleted events. |
| [common/webhook-viewer.png](screenshots/common/webhook-viewer.png) | Monitoring and external services | Webhook viewer: received event deliveries (a demo receiver on localhost). |
| [common/nats.png](screenshots/common/nats.png) | Monitoring and external services | NATS JetStream: the DATASAFE_EVENTS stream with event messages (terminal output). |
| [common/vault.png](screenshots/common/vault.png) | Monitoring and external services | Vault Transit: the bucket encryption key and Vault status (terminal output, token hidden). |
| [common/minio-src.png](screenshots/common/minio-src.png) | Monitoring and external services | MinIO as a migration source: objects of the test bucket (terminal output). |
| [common/keycloak-login.png](screenshots/common/keycloak-login.png) | LDAP and OIDC (Keycloak) sign-in | Keycloak: the datasafe realm sign-in page shown when signing in to the console via OIDC. |

Russian-only frames (wizard, LDAP/OIDC settings, tenant members, etc.) are listed in [images/README (RU)](../ru/images/README.md).

The set is not regenerated by scripts: the legacy capture scripts (`scripts/capture-screenshots.mjs`, `scripts/screenshots/capture.mjs`) write to `docs/images/_capture-output/` and never overwrite the published files.

## Diagrams

Architecture diagrams use **Mermaid** embedded in Markdown. See [diagrams/README.md](../diagrams/README.md).
