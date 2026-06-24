English | **[Русский](../../ru/specs/product-documentation-tz.md)**

# Product documentation — technical specification

**Project:** DataSafeS3  
**Audience:** Technical writers, solution architects, contributors

## Goal

Rebuild README and documentation around DataSafeS3 as a **standalone product**. Documentation must answer:

- What is DataSafeS3?
- What problems does it solve?
- Who is it for?
- How to deploy, configure, operate, and scale it?
- Why do organizations choose DataSafeS3?

Documentation must **not** answer: “How is DataSafeS3 better than product X?”

## Core narrative

**DataSafeS3** — self-hosted storage platform for secure and governed data management.

Value pillars: data control · security · access management · storage governance · enterprise integration · observability · data lifecycle.

## Forbidden in README and product docs

- Competitor comparisons and comparison tables
- Mentions of MinIO, Ceph, Nextcloud, SeaweedFS, etc. as positioning references
- Negative positioning; phrases “better than”, “alternative to”, “replacement for”, “instead of”

Allowed: describing DataSafeS3 capabilities and outcomes only.

## README structure (implemented)

1. Title + subtitle — product exists for governed self-hosted storage  
2. 2–3 paragraphs — problem, audience, value (no deep tech)  
3. Value capabilities — Secure Storage, Identity Integration, Governance, Auditability, Collaboration  
4. Product screenshots — Dashboard, storage, users, audit, monitoring  
5. Use cases — six scenarios with Problem / Solution / Result  
6. Architecture overview — conceptual diagram  
7. Quick start — ~5 minutes to first bucket  

## Documentation model (implemented in `docs/README.md`)

| Phase | Topics |
|-------|--------|
| **Learn** | Overview, concepts, architecture, security model, data model |
| **Deploy** | Docker, Compose, Kubernetes, Helm, production |
| **Configure** | Storage, LDAP, OIDC, MFA, monitoring |
| **Manage** | Users, roles, tenants, buckets, policies, lifecycle, replication, quotas |
| **Operate** | Backup, restore, monitoring, troubleshooting, upgrade, DR |
| **Reference** | Configuration, env vars, permissions, events, metrics |
| **API** | OpenAPI, Swagger, curl guide |

## Architecture deliverables

See [Conceptual architecture](../../learn/en/conceptual-architecture.md):

- Conceptual (30-second view)
- Logical subsystems
- Single-node deployment
- Production (Kubernetes)
- Multi-site (Gateway replication)
- Authentication (LDAP, OIDC, MFA)

## Writing style

| Avoid | Prefer |
|-------|--------|
| “DataSafeS3 supports LDAP.” | “Integrate existing corporate directories through LDAP.” |
| “Supports audit logging.” | “Track administrative and user activity for operational visibility and compliance.” |
| “OIDC authentication.” | “Enable single sign-on using your organization's identity provider.” |

## Use case format

Each scenario: **Problem** → **Solution** → **Result**

## Acceptance criteria

After reading docs, a new user understands:

1. Why DataSafeS3 exists  
2. Business problems it solves  
3. How to start quickly  
4. How to deploy in an organization  
5. How to operate safely  
6. How to scale  

Success = desire to adopt DataSafeS3 for **its own value**, not because of comparisons.

## Implementation status

| Item | Location |
|------|----------|
| Root README | `/README.md` |
| Documentation hub | `/docs/README.md` |
| Why DataSafeS3 | `/docs/en/why-datasafe.md`, `/docs/ru/why-datasafe.md` |
| Conceptual architecture | `/docs/learn/en/`, `/docs/learn/ru/` |
| Use cases | `/docs/use-cases/` |

Technical runbooks (Gateway test endpoints, integration guides) may reference external S3 endpoints for lab setup; product positioning docs must not use third-party product names for comparison.
