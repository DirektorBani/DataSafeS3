English | **[Русский](../ru/conceptual-architecture.md)**

# Conceptual architecture

High-level architecture for product understanding. For implementation detail see [logical architecture](../../en/context/architecture.md) and [database schema](../../en/database.md).

---

## Conceptual architecture (30 seconds)

```mermaid
flowchart TB
  people[Users and applications]
  ds[DataSafeS3 platform]
  disks[Your storage]
  people --> ds --> disks
```

DataSafeS3 sits between people (browser, S3 clients, automation) and disk storage. Identity, policy, audit, and monitoring wrap every request.

---

## Logical architecture

```mermaid
flowchart TB
  clients[Browser S3 CLI SDK]
  caddy[Caddy reverse proxy]
  console[Web console]
  server[storage-server]
  subgraph services [Core services]
    s3[S3 API SigV4]
    admin[Admin REST API]
    gw[Gateway worker]
  end
  meta[(Metadata store)]
  objects[Object files on disk]
  clients --> caddy & server
  caddy --> console
  caddy --> server
  server --> s3 & admin & gw
  server --> meta & objects
```

| Subsystem | Role |
|-----------|------|
| **Web console** | Administration and self-service for users |
| **storage-server** | S3 API, Admin API, replication worker, metrics |
| **Metadata store** | Users, buckets, policies, tenants, audit pointers |
| **Object store** | Content-addressed files under `STORAGE_DATA_DIR/objects/` |

---

## Deployment architecture — single node

```mermaid
flowchart LR
  subgraph host [Single host Docker Compose]
    caddy[Caddy :8080]
    srv[storage-server :9000]
    bolt[(BoltDB default)]
    prom[Prometheus]
    graf[Grafana]
  end
  disk[(Local disk volume)]
  srv --> bolt & disk
  prom --> srv
  graf --> prom
```

Typical evaluation and small-team production: one VM or server, Compose stack, optional PostgreSQL profile for metadata.

Guide: [First run](../../getting-started/en/first-run.md)

---

## Production architecture

```mermaid
flowchart TB
  subgraph k8s [Kubernetes cluster]
    ingConsole[Ingress console]
    ingS3[Ingress S3 API]
    pods[DataSafeS3 pods]
    pg[(PostgreSQL StatefulSet)]
    mon[Prometheus Grafana]
  end
  pvc[(PersistentVolume objects)]
  pods --> pg & pvc
  ingConsole & ingS3 --> pods
  mon --> pods
```

Production checklist: TLS, PostgreSQL metadata, backups, monitoring alerts, changed bootstrap credentials. Optional HA: [2-node reference](../../operations-guide/en/reference-deployment-2node.md), Helm `values-ha.yaml`.

Guide: [Helm chart](../../../deploy/helm/datasafe/README.md) · [Operations guide](../../operations-guide/en/README.md)

---

## Multi-site architecture — Gateway replication

```mermaid
flowchart LR
  primary[Primary DataSafeS3 site]
  gateway[Gateway async worker]
  remote[External S3-compatible site]
  primary --> gateway --> remote
```

Primary site accepts writes locally. Gateway replicates objects to an external bucket for off-site retention or DR.

Guide: [Replication](../../administrator-guide/en/replication.md) · [Gateway technical doc](../../en/context/gateway.md)

---

## Authentication architecture

```mermaid
flowchart TB
  subgraph consoleLogin [Console sign-in]
    local[Local password]
    ldap[LDAP directory]
    oidc[OIDC identity provider]
  end
  jwt[JWT session]
  mfa[MFA TOTP optional]
  local & ldap & oidc --> jwt
  jwt --> mfa
  s3[S3 API access keys SigV4]
  apps[Applications] --> s3
```

| Path | Use case |
|------|----------|
| **LDAP** | Corporate directory sync and group mapping |
| **OIDC / SSO** | Single sign-on with external IdP |
| **MFA** | Second factor for console accounts |
| **S3 keys** | Application and automation access to object API |

Guides: [LDAP](../../administrator-guide/en/ldap.md) · [OIDC](../../administrator-guide/en/oidc.md) · [MFA](../../administrator-guide/en/mfa.md)

---

## Related

- [What is DataSafeS3?](../../getting-started/en/what-is-datasafe.md)
- [Why DataSafeS3?](../../en/why-datasafe.md)
- [Use cases](../../use-cases/README.md)
