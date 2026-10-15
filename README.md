# DataSafeS3 · Датасейф S3

**[English](docs/en/README.md)** · **[Русский](docs/ru/README.md)** · **[Documentation](docs/README.md)**

**Author / Автор:** Ilya Trachuk · **License / Лицензия:** [Apache-2.0](LICENSE) · **Release / Релиз:** [v1.6.0](CHANGELOG.md#160---2026-10-15)

---

## Self-hosted storage platform for secure and governed data management

**Платформа локального хранения для безопасного управления данными и политиками доступа**

---

Organizations accumulate files, backups, media, and application data that must stay under their own control. DataSafeS3 gives teams a single platform to **store data securely**, **integrate corporate identity**, **govern lifecycle and quotas**, and **observe operations** — without depending on a public cloud vendor for administration or policy.

DataSafeS3 is built for **IT administrators**, **platform engineers**, **security teams**, and **business units** that need governed object storage on their infrastructure: on a single server, in Docker Compose, or in Kubernetes with Helm.

The product combines **S3-compatible storage**, a **web console**, **enterprise authentication**, and **operational tooling** in one self-hosted solution.

| | |
|---|---|
| **Secure storage** | Centralized access control, encryption options, object lifecycle |
| **Identity integration** | LDAP, OIDC/SSO, MFA for privileged accounts |
| **Governance** | Tenants, quotas, retention, replication policies |
| **Auditability** | Activity log and external log sinks |
| **Collaboration** | Personal web workspace, bucket/folder sharing, presigned links; desktop sync (`datasafe-sync` CLI + optional Tauri UI) |
| **Observability** | Prometheus metrics and Grafana dashboards |
| **Production trust** | GHCR release images, SBOM, cosign signing, nightly regression CI |
| **High availability lab** | Erasure object backend, Postgres metadata HA + leader lock, site replication, **trusted cluster pairing**, failover scripts. CE lab foundation, not production multi-AZ — [HA reference](docs/operations-guide/en/reference-deployment-2node.md) |

Подробнее о ценности продукта: [Why DataSafeS3](docs/why-datasafe.md) · [RU](docs/ru/why-datasafe.md)

---

## Product screenshots

| Dashboard / Обзор | Storage / Бакеты | Object browser / Объекты |
|-------------------|------------------|--------------------------|
| ![Dashboard](docs/images/screenshots/dashboard.png) | ![Buckets](docs/images/screenshots/buckets.png) | ![Object browser](docs/images/screenshots/object-browser.png) |

| User management / Пользователи | Audit / Активность | Monitoring / Мониторинг |
|------------------------------|--------------------|-------------------------|
| ![Users](docs/images/screenshots/users.png) | ![Activity](docs/images/screenshots/activity.png) | ![Monitoring](docs/images/screenshots/monitoring.png) |

| Tenants / Тенанты | Settings / Настройки | Gateway / Репликация |
|-------------------|----------------------|----------------------|
| ![Tenants](docs/images/screenshots/tenants.png) | ![Settings](docs/images/screenshots/settings.png) | ![Gateway](docs/images/screenshots/gateway.png) |

---

## Use cases

| Scenario | Description |
|----------|-------------|
| [Enterprise file storage](docs/use-cases/en/corporate-file-storage.md) | Shared storage with SSO, tenants, and audit |
| [Backup repository](docs/use-cases/en/backup-storage.md) | Durable target for backups and snapshots |
| [Private cloud storage](docs/use-cases/en/document-storage.md) | Internal document and content repository |
| [Document archive](docs/use-cases/en/data-archive.md) | Long-term retention and lifecycle policies |
| [Media repository](docs/use-cases/en/media-storage.md) | Large media assets with S3 access |
| [Kubernetes object storage](docs/use-cases/en/k8s-object-storage.md) | S3 backend for clusters and CI/CD |

[All use cases / Все сценарии](docs/use-cases/README.md)

---

## Architecture overview

High-level view — no implementation detail. Deep dives: [Conceptual architecture](docs/learn/en/conceptual-architecture.md) · [RU](docs/learn/ru/conceptual-architecture.md)

```mermaid
flowchart TB
  users[Users / Пользователи]
  subgraph datasafe [DataSafeS3]
    console[Web console]
    api[Storage and Admin API]
    gov[Governance and audit]
  end
  storage[(Object storage)]
  identity[Identity LDAP OIDC MFA]
  monitoring[Monitoring Prometheus Grafana]
  database[(Metadata database)]
  external[External S3 optional replication]

  users --> console & api
  api --> storage & database
  api --> gov
  identity -.-> console & api
  monitoring --> api
  api -.-> external
```

Kafka is an opt-in event sink. A default install does not set `STORAGE_KAFKA_BROKERS` and does not start a broker. The process opens no Kafka connection unless that variable is non-empty. Installer profile `kafka` is not one of the default profiles.

---

## v1.6.0

- SSE-S3 and optional SSE-KMS (Vault Transit). A single node with no Vault does not open a KMS connection.
- Key rotation, rewrap, and crypto-shred. Finish the rolling upgrade of every node before enabling encryption.
- `storage-cli migrate from-minio` copies objects and IAM. **Settings → Migrate** prints the commands and opens a local evidence file. It does not store source credentials.
- An unversioned Postgres overwrite is one transaction. v1.5.0 could drop the metadata row if the process died between `DELETE` and `INSERT`.
- The resource budget compares this build with the published v1.5.0 image. This commit does not push the git tag.

Upgrade: [EN](docs/operations-guide/en/upgrade.md#upgrading-to-v160) · [RU](docs/operations-guide/ru/upgrade.md#обновление-до-v160)

## v1.5.0

- Durable outbox: at-least-once delivery, per-object order per sink, and a dead-letter row after retries are exhausted.
- Webhooks, NATS JetStream, and optional Kafka (`STORAGE_KAFKA_BROKERS`, sidecar `/usr/lib/datasafe/datasafe-kafka-bridge`) are an external event feed. They are not how storage nodes talk to each other.
- Console page **Settings → Event delivery**, a Grafana **Event delivery** row, and example alerts.
- Scripted Postgres promote and a DR evidence pack. A follower takes the storage leader lease after it expires. Takeover time is about that lease.
- A gateway replication rule can set its own sync schedule. Erasure and site-A lab ports no longer share the HA ports. Delete an old `.env.erasure` or `.env.site-a` so the new ports apply.

Upgrade: [EN](docs/operations-guide/en/upgrade.md#upgrading-to-v150) · [RU](docs/operations-guide/ru/upgrade.md#обновление-до-v150)

---

## Quick start

**Interactive installer** (recommended):

```powershell
# Windows
.\install.ps1
```

```bash
# Linux / macOS / WSL
chmod +x install.sh && ./install.sh
```

Or `install.cmd` on Windows. Menu: Core, Postgres, Monitoring, host data, optional build / Identity lab / Kafka. Kafka is opt-in only and is not part of the default profile set.  
Docs: [EN](docs/getting-started/en/installer.md) · [RU](docs/getting-started/ru/installer.md)

**Manual** (equivalent default profile):

```cmd
copy .env.example .env
docker compose -p datasafe --profile postgres -f docker-compose.yml -f docker-compose.local-data.yml up -d
```

The default Compose stack serves the console from `DATASAFE_CONSOLE_IMAGE` (`ghcr.io/direktorbani/datasafe-console:v1.6.0`); a host `web/console/dist` is not required. To serve a local production build, add `-f deploy/compose/docker-compose.local-console.yml`. For Vite HMR during UI work: add `-f deploy/compose/docker-compose.dev.yml --profile dev`.

Published images (on release tags): `ghcr.io/direktorbani/datasafe-storage-server:v1.6.0` and `ghcr.io/direktorbani/datasafe-console:v1.6.0`.

Default Compose install is production-safe: `STORAGE_DEV` is off unless you pass installer `-Dev` / `--dev` (or profile `dev`). The installer generates `STORAGE_METRICS_TOKEN` and wires Prometheus. First login is still `admin` / `admin`, then the setup wizard.

**Rolling `main` builds** (for testers, not cosign/SBOM): on each push to `main`, [`.github/workflows/publish-main.yml`](.github/workflows/publish-main.yml) publishes `:main` and `:sha-<commit>` tags for both images, e.g. `ghcr.io/direktorbani/datasafe-storage-server:main`. Versioned releases (`v*`) remain the supported production path with SBOM and cosign signatures.

1. Open **http://localhost:8080** → sign in `admin` / `admin`
2. Change the administrator password
3. Complete the **setup wizard** (external S3 is optional)
4. Create a bucket in the console or via S3 API

| Service | URL | Default credentials |
|---------|-----|-------------------|
| Console | http://localhost:8080 | `admin` / `admin` (change on first login) |
| S3 API | http://localhost:9000 | `datasafe` / `datasafesecret` |
| Grafana | http://localhost:3000 | `admin` / `admin` |

Step-by-step onboarding: [EN](docs/getting-started/en/onboarding.md) · [RU](docs/getting-started/ru/onboarding.md)

Production: [Deploy guide](docs/README.md#deploy) · [Helm chart](deploy/helm/datasafe/README.md)

---

## Documentation

Documentation is organized by what you need to do:

| Phase | Topics |
|-------|--------|
| **Learn** | [Overview](docs/getting-started/en/what-is-datasafe.md), [architecture](docs/learn/en/conceptual-architecture.md), [security model](docs/administrator-guide/en/mfa.md) |
| **Deploy** | [Docker Compose](docs/getting-started/en/first-run.md), [Kubernetes / Helm](deploy/helm/datasafe/README.md) |
| **Configure** | [LDAP](docs/administrator-guide/en/ldap.md), [OIDC](docs/administrator-guide/en/oidc.md), [monitoring](docs/administrator-guide/en/monitoring.md) |
| **Manage** | [Users](docs/administrator-guide/en/users.md), [tenants](docs/administrator-guide/en/tenants.md), [replication](docs/administrator-guide/en/replication.md) |
| **Operate** | [Backup](docs/operations-guide/en/backup-restore.md), [Migrate from MinIO](docs/operations-guide/en/migrate-from-minio.md), [HA reference](docs/operations-guide/en/reference-deployment-2node.md), [scaling](docs/operations-guide/en/scaling.md), [troubleshooting](docs/operations-guide/en/troubleshooting.md) |
| **Integrate** | [Partner cookbook](docs/operations-guide/en/partner-cookbook.md), [extension hooks](examples/extension-hook/README.md) |
| **Reference** | [Database schema](docs/en/database.md), [Gateway](docs/en/context/gateway.md) |
| **API** | [API guide](docs/api-guide/en/README.md) · Swagger `/api/v1/docs` |

Full hub: **[docs/README.md](docs/README.md)**

---

## Repository layout

| Path | Purpose |
|------|---------|
| `cmd/storage-server` | HTTP server (S3 + Admin API) |
| `cmd/datasafe-sync` | Desktop folder sync CLI (file collaboration Phase 3) |
| `clients/desktop` | Tauri desktop shell |
| `clients/mobile` | Flutter mobile app (Phase 4) |
| `clients/mobile-web` | Mobile PWA preview |
| `web/console` | React web console |
| `internal/metadata` | BoltDB and PostgreSQL metadata |
| `deploy/helm/datasafe` | Kubernetes Helm chart |
| `deploy/compose/` | Optional Compose overlays (HA, Vault, OAuth2, audit, …) |
| `docs/` | Product documentation |

---

## License

Apache-2.0 · Copyright © Ilya Trachuk
