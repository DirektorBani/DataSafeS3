English | **[Русский](../ru/why-datasafe.md)**

# Why DataSafeS3?

DataSafeS3 exists so organizations can **manage data, users, and storage governance from a single self-hosted platform** — with full control over where bytes live and who can access them.

---

## Data under your control

Store objects on infrastructure you operate. Metadata lives in BoltDB or PostgreSQL on your network. Standard S3 APIs let applications integrate without rewriting storage logic. Apache-2.0 source code supports audit, customization, and long-term ownership of the platform.

---

## Secure storage

Protect data with role-based access, tenant isolation, bucket policies, and optional server-side encryption. Object Lock and legal hold support retention requirements. Soft delete and lifecycle rules help enforce how long data is kept.

---

## Identity integration

Connect existing corporate directories through **LDAP**. Enable **single sign-on** using your organization's identity provider via **OIDC**. Require **MFA** for privileged administrator accounts. Users sign in once; access follows corporate identity policies.

---

## Governance

Isolate departments or customers with **tenants** and **groups**. Set **quotas** on users and buckets. Define **lifecycle** rules for expiration and transition. Delegate administration with tenant-scoped roles without sharing global keys.

---

## Auditability

Track administrative and user activity for operational visibility and compliance. Stream structured logs to Syslog, Loki, Elasticsearch, or webhooks alongside the built-in activity log in the console.

---

## Collaboration

Give users a **personal file workspace** in the web console (home bucket, **My files** / **Shared with me**) and let owners share buckets or folders with colleagues. External partners still use **presigned share links** with expiry and download limits.

Optional **desktop folder sync** via `datasafe-sync` CLI and Tauri desktop app (Community Edition). Mobile clients are on the [Phase 4 backlog](specs/file-collaboration-phase4-backlog.md). Details: [File collaboration status](specs/file-collaboration-status.md).

---

## Observability

Monitor health and capacity with Prometheus metrics and prebuilt Grafana dashboards. Gateway replication queues, storage growth, and API latency are visible from day one in Docker Compose or Kubernetes.

---

## Replication and resilience

Use **Gateway** to replicate objects asynchronously to external S3-compatible storage for off-site copies and disaster recovery — without blocking application writes to the primary site.

For metadata and console availability, Community Edition documents **PostgreSQL streaming replication**, **read-only storage-server standby**, and **failover scripts** — see [2-node reference](../operations-guide/en/reference-deployment-2node.md).

Scoped application credentials: **STS AssumeRole** returns short-lived session tokens for S3 SigV4 — **bound to the authenticated user** who called the endpoint.

---

## Production readiness

| Signal | What it means |
|--------|----------------|
| **GHCR images + SBOM** | Reproducible releases with supply-chain metadata |
| **cosign signatures** | Verify container images on deploy |
| **Nightly feature-audit CI** | 93+ automated regression checks on `main` |
| **Published benchmarks** | [performance-benchmarks](../testing/performance-benchmarks.md) with list-index improvements |
| **SECURITY.md** | Coordinated disclosure process |

## Who DataSafeS3 is for

| Audience | Value |
|----------|-------|
| **IT administrators** | One console for users, tenants, policies, and audit |
| **Platform / DevOps engineers** | S3 endpoint for apps, CI/CD, and Kubernetes |
| **Security & compliance** | Self-hosted data path, MFA, activity log, retention |
| **Business units & MSPs** | Multi-tenant isolation with delegated administration |

---

## Next steps

- [What is DataSafeS3?](../getting-started/en/what-is-datasafe.md)
- [Onboarding checklist](../getting-started/en/onboarding.md)
- [Use cases](../use-cases/README.md)
- [Quick start](../../README.md#quick-start)
