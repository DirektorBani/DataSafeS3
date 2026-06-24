English | **[Русский](../ru/what-is-datasafe.md)**

# What is DataSafeS3?

**DataSafeS3** is a self-hosted storage platform for secure and governed data management.

It combines **S3-compatible object storage**, a **web console**, **enterprise authentication**, and **operational tooling** so organizations can store data, control access, and observe usage on infrastructure they operate.

---

## What problem does DataSafeS3 solve?

Teams need a place to put backups, documents, media, and application blobs that:

- stays on infrastructure the organization controls;
- integrates with corporate identity (LDAP, SSO, MFA);
- provides audit trails and quotas;
- speaks the de-facto S3 API expected by modern software.

DataSafeS3 addresses that need as a **single product** — not a loose collection of scripts around raw disk.

---

## Who is it for?

| Audience | Typical need |
|----------|--------------|
| **IT administrators** | Govern users, tenants, policies, and audit from one console |
| **Platform engineers** | Offer S3 storage to Kubernetes, CI/CD, and internal apps |
| **Security teams** | Keep data path and logs on-prem or in chosen regions |
| **Business units / MSPs** | Isolate customers or departments with multi-tenancy |

---

## Core capabilities (value view)

| Capability | Outcome |
|------------|---------|
| **Secure storage** | Store data with centralized access control |
| **Identity integration** | Connect existing identity providers |
| **Governance** | Manage lifecycle, quotas, and retention |
| **Auditability** | Track administrative and user activity |
| **Collaboration** | Personal file workspace, bucket sharing between users, and controlled share links |
| **Observability** | Monitor health, capacity, and replication |

---

## How it is delivered

- **Single binary** `storage-server` — S3 API + Admin API + Gateway worker
- **Web console** — React UI with **EN / RU / DE / FR** localization
- **Docker Compose** — fastest path to evaluate and small production; optional [HA overlay](../../operations-guide/en/reference-deployment-2node.md)
- **Helm chart** — Kubernetes production deployments (`values-production.yaml`, `values-ha.yaml`)
- **Metadata** — BoltDB (default) or PostgreSQL with read-replica routing
- **Release artifacts** — `ghcr.io/direktorbani/datasafe-*` images, SBOM, cosign signatures on tags
- **Integrations** — STS scoped credentials, NATS event sink, [extension hooks](../../../examples/extension-hook/README.md)
- **File workspace** — home bucket, sharing, optional [desktop sync CLI](../../../clients/README.md) — [status](../../en/specs/file-collaboration-status.md)

---

## Next steps

- [Why DataSafeS3?](../../en/why-datasafe.md)
- [Conceptual architecture](../../learn/en/conceptual-architecture.md)
- [Onboarding](onboarding.md)
- [Use cases](../../use-cases/README.md)
