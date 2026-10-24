# What it looks like

The screenshots were taken on a real DataSafeS3 v1.7.0 installation from the images published on GitVerse (Windows 11 + Docker Desktop, `docker compose`) with every integration enabled: LDAP, Keycloak (OIDC), Kafka, NATS JetStream, webhooks, HashiCorp Vault (KMS), Grafana and Prometheus, a second site and a MinIO source. All data is fictional: users, buckets and files were created for the demo, addresses are `localhost` only, and no secrets or passwords are shown. Some frames are terminal screenshots rendered from the real command output. The gallery is shown in English; screens that exist only in Russian are listed in the Russian gallery.

Use the gallery to see the interface before you install anything. For the screens themselves see the [user guide](user-guide/README.md) and the [administrator guide](../administrator-guide/en/README.md).

## Installation

### Clone and install.ps1

![DataSafeS3 — Clone and install.ps1](../images/screenshots/common/install-clone.png)

*Cloning the repository and running install.ps1: Docker and port checks and profile selection (terminal output, Windows 11 + Docker Desktop).*

### LDAP and Keycloak lab

![DataSafeS3 — LDAP and Keycloak lab](../images/screenshots/common/install-identity.png)

*The installer starts the test LDAP and Keycloak and prints the lab endpoints and credentials (terminal output).*

### docker compose up

![DataSafeS3 — docker compose up](../images/screenshots/common/install-compose.png)

*docker compose up from the published GitVerse images and the console readiness check (terminal output).*

### Core services

![DataSafeS3 — Core services](../images/screenshots/common/compose-ps-core.png)

*Base installation: storage server, Caddy, PostgreSQL, Prometheus, Grafana and Kafka are all running (docker compose ps).*

### All integrations

![DataSafeS3 — All integrations](../images/screenshots/common/compose-ps-full.png)

*Full installation with every integration: two storage nodes, Kafka and Kafka UI, NATS, Vault, a MinIO source, a second site and a webhook viewer (docker ps).*

## Console: overview, buckets and objects

### Sign-in

![DataSafeS3 — Sign-in](../images/screenshots/en/login.png)

*Console sign-in page with the language switcher at the top right and the SSO option.*

### Dashboard

![DataSafeS3 — Dashboard](../images/screenshots/en/dashboard.png)

*Overview: buckets, storage used, objects, access keys and API status (dark theme, demo data).*

### Buckets

![DataSafeS3 — Buckets](../images/screenshots/en/buckets.png)

*Bucket list: demo buckets with different access levels (dark theme).*

### Object browser

![DataSafeS3 — Object browser](../images/screenshots/en/object-browser.png)

*Object browser: folders, upload, share links and the metadata panel of the selected object (dark theme).*

### Object versions

![DataSafeS3 — Object versions](../images/screenshots/en/object-versions.png)

*Object versions: each version has an ID, size and date; a version can be downloaded or deleted.*

### Lifecycle

![DataSafeS3 — Lifecycle](../images/screenshots/en/bucket-lifecycle.png)

*Bucket lifecycle rules: automatic deletion of objects by prefix after a set number of days.*

### Bucket access

![DataSafeS3 — Bucket access](../images/screenshots/en/bucket-access.png)

*Bucket settings, Access tab: tenant access and access to folders (prefixes) inside the bucket.*

### File sharing

![DataSafeS3 — File sharing](../images/screenshots/en/bucket-sharing.png)

*Bucket settings, File sharing tab: public access, share links and their lifetime.*

### Bucket encryption

![DataSafeS3 — Bucket encryption](../images/screenshots/en/bucket-encryption.png)

*Bucket encryption: server key or Vault Transit key (the secure-vault bucket).*

## Access, users and audit

### Access keys

![DataSafeS3 — Access keys](../images/screenshots/en/keys.png)

*S3 access keys and API tokens; secret keys are not displayed.*

### Usage

![DataSafeS3 — Usage](../images/screenshots/en/usage.png)

*Usage: size and object count per bucket and storage growth.*

### Users

![DataSafeS3 — Users](../images/screenshots/en/users.png)

*Users: roles, quotas, statuses and last sign-in (fictional users).*

### Teams

![DataSafeS3 — Teams](../images/screenshots/en/teams.png)

*Teams: grouping users for access grants.*

### Tenants

![DataSafeS3 — Tenants](../images/screenshots/en/tenants.png)

*Tenants: isolated organisations with their own users and buckets.*

### Activity

![DataSafeS3 — Activity](../images/screenshots/en/activity.png)

*Activity log: sign-ins, administrator actions and delivery events (filters and CSV/JSON export).*

### Webhooks

![DataSafeS3 — Webhooks](../images/screenshots/en/webhooks.png)

*Webhooks: object event subscriptions (a delivery viewer is attached).*

### Profile and MFA

![DataSafeS3 — Profile and MFA](../images/screenshots/en/profile.png)

*Profile: two-factor authentication (TOTP), passkeys (WebAuthn) and password change.*

## Settings and integrations

### Settings

![DataSafeS3 — Settings](../images/screenshots/en/settings.png)

*Administrator settings: soft delete, LDAP/AD and OIDC/SSO.*

### Bucket settings

![DataSafeS3 — Bucket settings](../images/screenshots/en/settings-buckets.png)

*Bucket settings: apply parameters to several buckets at once.*

### Security

![DataSafeS3 — Security](../images/screenshots/en/settings-security.png)

*Security: public access, pre-signed links, lifetimes and MFA.*

### Event delivery

![DataSafeS3 — Event delivery](../images/screenshots/en/settings-events.png)

*Event delivery: Kafka, NATS JetStream and webhooks, with queue and delivery counters.*

### Vault (KMS)

![DataSafeS3 — Vault (KMS)](../images/screenshots/en/settings-kms.png)

*Vault (KMS): connection to Vault Transit for encryption; the token is not displayed.*

### MinIO migration

![DataSafeS3 — MinIO migration](../images/screenshots/en/settings-migrate.png)

*Migration from MinIO: the wizard builds the command for the migration utility.*

## Gateway, replication and cluster

### Gateway

![DataSafeS3 — Gateway](../images/screenshots/en/gateway.png)

*Gateway: connections to an external S3 storage (the Site B demo site) with a connection test, and the add-connection form.*

### Replication rules

![DataSafeS3 — Replication rules](../images/screenshots/en/gateway-rules.png)

*Gateway: replication rules and manual sync.*

### Sync jobs

![DataSafeS3 — Sync jobs](../images/screenshots/en/gateway-jobs.png)

*Gateway: sync jobs and their results.*

### Gateway status

![DataSafeS3 — Gateway status](../images/screenshots/en/gateway-status.png)

*Gateway: status — 2/2 connections OK, one rule, empty replication queue, 4 tasks replicated.*

### Clusters

![DataSafeS3 — Clusters](../images/screenshots/en/cluster.png)

*Clusters: the local node and trusted clusters.*

### HA status

![DataSafeS3 — HA status](../images/screenshots/common/ha-status.png)

*Two-node HA: /cluster/status reports HA enabled and a leader elected (terminal output).*

## Monitoring and external services

### Grafana: overview

![DataSafeS3 — Grafana: overview](../images/screenshots/common/monitoring.png)

*Grafana overview dashboard: requests, storage, object count, latency and HTTP errors (Prometheus data).*

### Grafana: buckets

![DataSafeS3 — Grafana: buckets](../images/screenshots/common/grafana-buckets.png)

*Grafana buckets dashboard: size and object count per bucket.*

### Prometheus

![DataSafeS3 — Prometheus](../images/screenshots/common/prometheus-targets.png)

*Prometheus: the storage-server scrape target is UP.*

### Kafka UI: topics

![DataSafeS3 — Kafka UI: topics](../images/screenshots/common/kafka-ui-topics.png)

*Kafka UI: the datasafe.events topic with storage events.*

### Kafka UI: messages

![DataSafeS3 — Kafka UI: messages](../images/screenshots/common/kafka-ui-messages.png)

*Kafka UI: messages of the datasafe.events topic: object created and deleted events.*

### Webhook viewer

![DataSafeS3 — Webhook viewer](../images/screenshots/common/webhook-viewer.png)

*Webhook viewer: received event deliveries (a demo receiver on localhost).*

### NATS JetStream

![DataSafeS3 — NATS JetStream](../images/screenshots/common/nats.png)

*NATS JetStream: the DATASAFE_EVENTS stream with event messages (terminal output).*

### Vault Transit

![DataSafeS3 — Vault Transit](../images/screenshots/common/vault.png)

*Vault Transit: the bucket encryption key and Vault status (terminal output, token hidden).*

### MinIO source

![DataSafeS3 — MinIO source](../images/screenshots/common/minio-src.png)

*MinIO as a migration source: objects of the test bucket (terminal output).*

## LDAP and OIDC (Keycloak) sign-in

### Keycloak sign-in

![DataSafeS3 — Keycloak sign-in](../images/screenshots/common/keycloak-login.png)

*Keycloak: the datasafe realm sign-in page shown when signing in to the console via OIDC.*

---

All files are in [images/screenshots/](../images/screenshots/); the list and captions are in [manifest.json](../images/screenshots/manifest.json). Catalogue description: [images/README.md](../images/README.md).
