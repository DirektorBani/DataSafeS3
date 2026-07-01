# DataSafeS3 Helm Chart

Helm chart for deploying **DataSafeS3** — self-hosted S3-compatible object storage with web console, Admin API, optional PostgreSQL metadata, LDAP/OIDC, Gateway replication, and Prometheus/Grafana monitoring.

**User guide:** [EN quick start](../../docs/en/user-guide/README.md#kubernetes--helm) · [RU](../../docs/ru/user-guide/README.md#kubernetes--helm)

## Prerequisites

- Kubernetes 1.24+
- Helm 3.10+
- PersistentVolume provisioner (for object data and optional PostgreSQL)
- Built images (or pull from GHCR):
  - `ghcr.io/direktorbani/datasafe-storage-server:v1.0.3` — release tag (or build locally: `docker build -f deploy/docker/Dockerfile -t datasafe/storage-server:latest .`)
  - `ghcr.io/direktorbani/datasafe-console:v1.0.3` — release tag (or build via `deploy/docker/Dockerfile.console`)

## Quick install (GHCR release tags)

```bash
# Minimal BoltDB stack with published v1.0.3 images
helm install datasafe deploy/helm/datasafe \
  --set storageServer.image.tag=v1.0.3 \
  --set console.image.tag=v1.0.3 \
  --namespace datasafe --create-namespace
```

## Quick install (local build)

```bash
# Minimal BoltDB stack (default values)
helm install datasafe deploy/helm/datasafe \
  --namespace datasafe --create-namespace

# Development overrides (smaller PVC, optional S3 test target)
helm install datasafe deploy/helm/datasafe \
  -f deploy/helm/datasafe/values-dev.yaml \
  --namespace datasafe --create-namespace

# Production-like with bundled PostgreSQL
# Hardened production overrides (secrets placeholders, STORAGE_STRICT_SECRETS): values-production.yaml
# HashiCorp Vault Agent Injector (optional): examples/values-vault-agent.yaml
helm install datasafe deploy/helm/datasafe \
  --set postgres.enabled=true \
  --set storageServer.metadataBackend=postgres \
  --namespace datasafe --create-namespace
```

Add to `/etc/hosts` (or configure DNS):

```
127.0.0.1 datasafe.local s3.datasafe.local
```

Port-forward if no Ingress controller:

```bash
kubectl port-forward -n datasafe svc/datasafe-caddy 8080:80
kubectl port-forward -n datasafe svc/datasafe-storage-server 9000:9000
```

## Upgrade / uninstall

```bash
helm upgrade datasafe deploy/helm/datasafe -f my-values.yaml -n datasafe
helm uninstall datasafe -n datasafe
```

## Building images

**storage-server** (from repository root):

```bash
docker build -f deploy/docker/Dockerfile -t datasafe/storage-server:latest .
```

**console** (static assets served by Caddy — no separate console Dockerfile):

```bash
# From repository root (Windows):
scripts/build-console.cmd

# Or manually:
cd web/console && npm ci && npm run build
```

Docker Compose and Helm mount `web/console/dist` into Caddy (same as `docker-compose.yml`). To package a console image for Kubernetes, copy `dist/` into any static-file base (e.g. `FROM caddy:2-alpine` with `COPY dist/ /srv/console/`).

The chart copies console static files from `console.image` into a shared `emptyDir` via an init container on the Caddy pod (same pattern as Docker Compose mounting `web/console/dist`).

## Architecture

| Component | Kubernetes resource | Port | Notes |
|-----------|---------------------|------|-------|
| storage-server | Deployment + Service | 9000 | S3 API, Admin API, `/metrics` |
| Caddy | Deployment + Service | 80 | Console + `/api/*` proxy |
| PostgreSQL | StatefulSet (optional) | 5432 | When `postgres.enabled=true` |
| Prometheus | Deployment (optional) | 9090 | Scrapes storage-server |
| Grafana | Deployment (optional) | 3000 | `datasafe-overview` dashboard |
| S3 test | Deployment (optional) | 9000/9001 | Dev-only Gateway test target |
| Ingress | 2× Ingress | — | Console host + S3 host |

## Key values

| Value | Default | Description |
|-------|---------|-------------|
| `storageServer.image.repository` | `datasafe/storage-server` | Main server image |
| `storageServer.replicaCount` | `1` | Replicas (single-node MVP) |
| `storageServer.metadataBackend` | `bolt` | `bolt` or `postgres` |
| `postgres.enabled` | `false` | Deploy PostgreSQL StatefulSet |
| `postgres.auth.*` | `datasafe` | DB credentials |
| `ldap.enabled` | `false` | Sets `STORAGE_LDAP_*` env vars |
| `oidc.enabled` | `false` | Sets `STORAGE_OIDC_*` env vars |
| `gateway.enabled` | `true` | Gateway worker env (`STORAGE_GATEWAY_*`) |
| `logging.*` | disabled | External sinks via system settings seed ConfigMap |
| `minio.enabled` | `false` | Dev S3 test target for Gateway testing |
| `caddy.enabled` | `true` | Web console reverse proxy |
| `monitoring.prometheus.enabled` | `true` | Prometheus deployment |
| `monitoring.grafana.enabled` | `true` | Grafana + dashboard |
| `ingress.enabled` | `true` | Console + S3 Ingress rules |
| `ingress.console.host` | `datasafe.local` | Console hostname |
| `ingress.s3.host` | `s3.datasafe.local` | S3 API hostname |
| `persistence.data.size` | `100Gi` | Object storage PVC |
| `persistence.metadata.enabled` | `false` | Separate BoltDB PVC |
| `systemSettings.seed.enabled` | `false` | ConfigMap with logging/LDAP JSON seed |

## Environment variable mapping

All `STORAGE_*` variables from `docker-compose.yml` and `.env.example` are mapped to ConfigMap/Secret on the storage-server Deployment:

| Compose / `.env` | Helm source |
|------------------|-------------|
| `STORAGE_ADDR` | `storageServer.config.addr` |
| `STORAGE_LOG_LEVEL` | `storageServer.config.logLevel` |
| `STORAGE_DATA_DIR` | `storageServer.config.dataDir` |
| `STORAGE_REGION` | `storageServer.config.region` |
| `STORAGE_ACCESS_KEY` | `storageServer.config.accessKey` |
| `STORAGE_SECRET_KEY` | `storageServer.config.secretKey` (Secret) |
| `STORAGE_ADMIN_USER` / `PASSWORD` | `storageServer.config.adminUser` / Secret |
| `STORAGE_JWT_SECRET` | Secret |
| `STORAGE_METADATA_BACKEND` | `metadataBackend` or auto `postgres` when `postgres.enabled` |
| `STORAGE_POSTGRES_HOST` | `release-postgres` service when `postgres.enabled` |
| `STORAGE_POSTGRES_*` | `storageServer.config.postgres.*` / `postgres.auth.*` |
| `STORAGE_SSE_MASTER_KEY` | Secret (optional) |
| `STORAGE_LDAP_*` | `ldap.*` when `ldap.enabled` |
| `STORAGE_OIDC_*` | `oidc.*` when `oidc.enabled` |
| `STORAGE_GATEWAY_*` | `storageServer.gateway.*` when `gateway.enabled` |

External logging (Syslog, Loki, Elasticsearch, Webhook) is configured in **system settings** (metadata DB), not env vars. Enable `systemSettings.seed.enabled` or `logging.seed` to create a reference ConfigMap, then apply via Admin → Settings or `PUT /api/v1/settings/system`.

## Example overrides

**LDAP + OIDC:**

```yaml
ldap:
  enabled: true
  url: ldap://openldap.example:389
  bindDn: cn=admin,dc=example,dc=local
  bindPassword: secret
  baseDn: ou=users,dc=example,dc=local

oidc:
  enabled: true
  issuer: https://keycloak.example/realms/datasafe
  internalIssuer: http://keycloak.keycloak.svc:8080/realms/datasafe
  clientId: datasafe-console
  clientSecret: secret
  redirectUrl: https://datasafe.example/api/v1/auth/oidc/callback
```

**External PostgreSQL (managed DB):**

```yaml
postgres:
  enabled: false
storageServer:
  metadataBackend: postgres
  config:
    postgres:
      host: my-rds.example.com
      port: "5432"
      user: datasafe
      password: secret
      database: datasafe
```

**Disable monitoring:**

```yaml
monitoring:
  prometheus:
    enabled: false
  grafana:
    enabled: false
```

**HashiCorp Vault (env injection, optional):**

See [operations guide — Vault](../../../docs/operations-guide/en/secrets-vault.md). Example overlay:

```bash
helm upgrade datasafe deploy/helm/datasafe \
  -f deploy/helm/datasafe/values-production.yaml \
  -f deploy/helm/datasafe/examples/values-vault-agent.yaml \
  -n datasafe
```

Requires Vault Agent Injector in the cluster. Clears inline Helm secret values; pod annotations render `STORAGE_*` via Agent templates.

## Validation

```bash
helm lint deploy/helm/datasafe
helm template datasafe deploy/helm/datasafe
helm template datasafe deploy/helm/datasafe --set postgres.enabled=true
helm template datasafe deploy/helm/datasafe --set ldap.enabled=true --set oidc.enabled=true
```

## License

Same as the DataSafeS3 repository.
