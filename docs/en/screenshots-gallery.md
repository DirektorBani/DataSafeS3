# What it looks like

The screenshots were taken on a real DataSafeS3 v1.7.0 installation from the images published on GitVerse (Windows 11 + Docker Desktop, `docker compose`) with every integration enabled: LDAP, Keycloak (OIDC), Kafka, NATS JetStream, webhooks, HashiCorp Vault (KMS), Grafana and Prometheus, a second site and a MinIO source. All data is fictional: users, buckets and files were created for the demo, addresses are `localhost` only, and no secrets or passwords are shown. Some frames are terminal screenshots rendered from the real command output. The gallery is shown in English; screens that exist only in Russian are listed in the Russian gallery.

Use the gallery to see the interface before you install anything. For the screens themselves see the [user guide](user-guide/README.md) and the [administrator guide](../administrator-guide/en/README.md).

## Installation

### Cloning the repository and running install.ps1

![Cloning the repository and running install.ps1](../images/screenshots/common/install-clone.png)

*Cloning the repository and running install.ps1: Docker and port checks and profile selection (terminal output, Windows 11 + Docker Desktop).*

### The installer starts the test LDAP and Keycloak and prints the lab endpoints and creden…

![The installer starts the test LDAP and Keycloak and prints the lab endpoints and credentials (terminal output)](../images/screenshots/common/install-identity.png)

*The installer starts the test LDAP and Keycloak and prints the lab endpoints and credentials (terminal output).*

### docker compose up from the published GitVerse images and the console readiness check (t…

![docker compose up from the published GitVerse images and the console readiness check (terminal output)](../images/screenshots/common/install-compose.png)

*docker compose up from the published GitVerse images and the console readiness check (terminal output).*

### Base installation

![Base installation](../images/screenshots/common/compose-ps-core.png)

*Base installation: storage server, Caddy, PostgreSQL, Prometheus, Grafana and Kafka are all running (docker compose ps).*

### Full installation with every integration

![Full installation with every integration](../images/screenshots/common/compose-ps-full.png)

*Full installation with every integration: two storage nodes, Kafka and Kafka UI, NATS, Vault, a MinIO source, a second site and a webhook viewer (docker ps).*

## Console: overview, buckets and objects

### Console sign-in page with the language switcher at the top right and the SSO option

![Console sign-in page with the language switcher at the top right and the SSO option](../images/screenshots/en/login.png)

*Console sign-in page with the language switcher at the top right and the SSO option.*

### Overview

![Overview](../images/screenshots/en/dashboard.png)

*Overview: buckets, storage used, objects, access keys and API status (dark theme, demo data).*

### Bucket list

![Bucket list](../images/screenshots/en/buckets.png)

*Bucket list: demo buckets with different access levels (dark theme).*

### Object browser

![Object browser](../images/screenshots/en/object-browser.png)

*Object browser: folders, upload, share links and the metadata panel of the selected object (dark theme).*

### Object versions

![Object versions](../images/screenshots/en/object-versions.png)

*Object versions: each version has an ID, size and date; a version can be downloaded or deleted.*

### Bucket lifecycle rules

![Bucket lifecycle rules](../images/screenshots/en/bucket-lifecycle.png)

*Bucket lifecycle rules: automatic deletion of objects by prefix after a set number of days.*

### Bucket settings, Access tab

![Bucket settings, Access tab](../images/screenshots/en/bucket-access.png)

*Bucket settings, Access tab: tenant access and access to folders (prefixes) inside the bucket.*

### Bucket settings, File sharing tab

![Bucket settings, File sharing tab](../images/screenshots/en/bucket-sharing.png)

*Bucket settings, File sharing tab: public access, share links and their lifetime.*

### Bucket encryption

![Bucket encryption](../images/screenshots/en/bucket-encryption.png)

*Bucket encryption: server key or Vault Transit key (the secure-vault bucket).*

## Access, users and audit

### S3 access keys and API tokens; secret keys are not displayed

![S3 access keys and API tokens; secret keys are not displayed](../images/screenshots/en/keys.png)

*S3 access keys and API tokens; secret keys are not displayed.*

### Usage

![Usage](../images/screenshots/en/usage.png)

*Usage: size and object count per bucket and storage growth.*

### Users

![Users](../images/screenshots/en/users.png)

*Users: roles, quotas, statuses and last sign-in (fictional users).*

### Teams

![Teams](../images/screenshots/en/teams.png)

*Teams: grouping users for access grants.*

### Tenants

![Tenants](../images/screenshots/en/tenants.png)

*Tenants: isolated organisations with their own users and buckets.*

### Activity log

![Activity log](../images/screenshots/en/activity.png)

*Activity log: sign-ins, administrator actions and delivery events (filters and CSV/JSON export).*

### Webhooks

![Webhooks](../images/screenshots/en/webhooks.png)

*Webhooks: object event subscriptions (a delivery viewer is attached).*

### Profile

![Profile](../images/screenshots/en/profile.png)

*Profile: two-factor authentication (TOTP), passkeys (WebAuthn) and password change.*

## Settings and integrations

### Administrator settings

![Administrator settings](../images/screenshots/en/settings.png)

*Administrator settings: soft delete, LDAP/AD and OIDC/SSO.*

### Bucket settings

![Bucket settings](../images/screenshots/en/settings-buckets.png)

*Bucket settings: apply parameters to several buckets at once.*

### Security

![Security](../images/screenshots/en/settings-security.png)

*Security: public access, pre-signed links, lifetimes and MFA.*

### Event delivery

![Event delivery](../images/screenshots/en/settings-events.png)

*Event delivery: Kafka, NATS JetStream and webhooks, with queue and delivery counters.*

### Vault (KMS)

![Vault (KMS)](../images/screenshots/en/settings-kms.png)

*Vault (KMS): connection to Vault Transit for encryption; the token is not displayed.*

### Migration from MinIO

![Migration from MinIO](../images/screenshots/en/settings-migrate.png)

*Migration from MinIO: the wizard builds the command for the migration utility.*

## Gateway, replication and cluster

### Gateway

![Gateway](../images/screenshots/en/gateway.png)

*Gateway: connections to an external S3 storage (the Site B demo site) with a connection test, and the add-connection form.*

### Gateway

![Gateway](../images/screenshots/en/gateway-rules.png)

*Gateway: replication rules and manual sync.*

### Gateway

![Gateway](../images/screenshots/en/gateway-jobs.png)

*Gateway: sync jobs and their results.*

### Gateway

![Gateway](../images/screenshots/en/gateway-status.png)

*Gateway: status — 2/2 connections OK, one rule, empty replication queue, 4 tasks replicated.*

### Clusters

![Clusters](../images/screenshots/en/cluster.png)

*Clusters: the local node and trusted clusters.*

### Two-node HA

![Two-node HA](../images/screenshots/common/ha-status.png)

*Two-node HA: /cluster/status reports HA enabled and a leader elected (terminal output).*

## Monitoring and external services

### Grafana overview dashboard

![Grafana overview dashboard](../images/screenshots/common/monitoring.png)

*Grafana overview dashboard: requests, storage, object count, latency and HTTP errors (Prometheus data).*

### Grafana buckets dashboard

![Grafana buckets dashboard](../images/screenshots/common/grafana-buckets.png)

*Grafana buckets dashboard: size and object count per bucket.*

### Prometheus

![Prometheus](../images/screenshots/common/prometheus-targets.png)

*Prometheus: the storage-server scrape target is UP.*

### Kafka UI

![Kafka UI](../images/screenshots/common/kafka-ui-topics.png)

*Kafka UI: the datasafe.events topic with storage events.*

### Kafka UI

![Kafka UI](../images/screenshots/common/kafka-ui-messages.png)

*Kafka UI: messages of the datasafe.events topic: object created and deleted events.*

### Webhook viewer

![Webhook viewer](../images/screenshots/common/webhook-viewer.png)

*Webhook viewer: received event deliveries (a demo receiver on localhost).*

### NATS JetStream

![NATS JetStream](../images/screenshots/common/nats.png)

*NATS JetStream: the DATASAFE_EVENTS stream with event messages (terminal output).*

### Vault Transit

![Vault Transit](../images/screenshots/common/vault.png)

*Vault Transit: the bucket encryption key and Vault status (terminal output, token hidden).*

### MinIO as a migration source

![MinIO as a migration source](../images/screenshots/common/minio-src.png)

*MinIO as a migration source: objects of the test bucket (terminal output).*

## LDAP and OIDC (Keycloak) sign-in

### Keycloak

![Keycloak](../images/screenshots/common/keycloak-login.png)

*Keycloak: the datasafe realm sign-in page shown when signing in to the console via OIDC.*

---

All files are in [images/screenshots/](../images/screenshots/); the list and captions are in [manifest.json](../images/screenshots/manifest.json). Catalogue description: [images/README.md](../images/README.md).
