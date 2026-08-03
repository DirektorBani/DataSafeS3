# Compose overlays

Canonical entrypoints stay at the **repository root**:

| File | Role |
|------|------|
| [`docker-compose.yml`](../../docker-compose.yml) | Core stack (storage-server, Caddy, Prometheus, Grafana; optional `postgres` profile) |
| [`docker-compose.local-data.yml`](../../docker-compose.local-data.yml) | Bind object + Postgres data to `DATASAFE_DATA_ROOT` on the host |
| [`.env.example`](../../.env.example) | Default env template → copy to `.env` |

Everything else lives here so the root stays clean for first-run and the future installer.

## Usage

Always run compose from the **repo root**. Paths inside these overlays are relative to `deploy/compose/`.

```bash
# Example: postgres + host data + local Linux binary
docker compose -p datasafe --profile postgres \
  -f docker-compose.yml \
  -f docker-compose.local-data.yml \
  -f deploy/compose/docker-compose.local-binary.yml \
  up -d
```

Env templates for labs: [`env/`](env/). Copy to a root working file (gitignored), e.g.:

```bash
cp deploy/compose/env/.env.ha.example .env.ha
```

## Overlay index

| Overlay | Purpose |
|---------|---------|
| `docker-compose.local-binary.yml` | Mount prebuilt `deploy/docker/storage-server-linux` |
| `docker-compose.dev.yml` | Vite HMR console (`--profile dev`) |
| `docker-compose.oauth2.yml` | oauth2-proxy edge SSO (`--profile oauth2`) |
| `docker-compose.audit.yml` | Feature-audit / CI (higher login rate limit, `STORAGE_DEV`) |
| `docker-compose.security-test.yml` | Stricter security QA defaults |
| `docker-compose.vault.yml` | HashiCorp Vault Agent lab (`--profile vault`) |
| `docker-compose.vault-product.yml` | Vault + production-like strict secrets |
| `docker-compose.ha.yml` | HA / read-only standby lab |
| `docker-compose.ha-local.yml` | Windows/local HA data paths |
| `docker-compose.ha-erasure.yml` | Erasure coding backend lab |
| `docker-compose.site-repl-lab.yml` | Site replication worker (site A) |
| `docker-compose.site-b.yml` | Second site ports/data (site B) |

Related: [`../docker/`](../docker/) (Caddy, Prometheus, entrypoints), [`../helm/datasafe/`](../helm/datasafe/), [`../vault/`](../vault/).
