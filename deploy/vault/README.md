# HashiCorp Vault — local development

Optional **opt-in** stack for the Vault Agent → `STORAGE_*` env injection pattern. DataSafeS3 has **no Vault SDK**; secrets are rendered to `/rendered/datasafe.env` and sourced at container start.

**Operations guide:** [EN](../../docs/operations-guide/en/secrets-vault.md) · [RU](../../docs/operations-guide/ru/secrets-vault.md)

## Prerequisites

- Docker Compose v2
- `.env` with `DATASAFE_DATA_ROOT` (Windows: `D:/datasafe-data`)
- Host dirs: `storage` and `postgres` under data root (if using `--profile postgres`)

## Quick start (Windows)

```powershell
cd D:\cursor_p
Copy-Item .env.vault.example .env -ErrorAction SilentlyContinue
# Ensure DATASAFE_DATA_ROOT=D:/datasafe-data

.\deploy\vault\local\setup-vault-dev.ps1

docker compose -p datasafe `
  -f docker-compose.yml `
  -f docker-compose.local-data.yml `
  -f docker-compose.vault.yml `
  --profile vault up -d
```

## Quick start (Linux / macOS)

```bash
chmod +x deploy/vault/local/setup-vault-dev.sh deploy/vault/init-kv.sh

./deploy/vault/local/setup-vault-dev.sh

docker compose -p datasafe \
  -f docker-compose.yml \
  -f docker-compose.vault.yml \
  --profile vault up -d
```

## Product-like strict secrets

```powershell
docker compose -p datasafe `
  -f docker-compose.yml `
  -f docker-compose.local-data.yml `
  -f docker-compose.vault.yml `
  -f docker-compose.vault-product.yml `
  --profile vault up -d
```

## Integration smoke (CI / manual)

```powershell
$env:DATASAFE_DATA_ROOT = 'D:/datasafe-data'
pwsh -File scripts/vault/smoke-vault-integration.ps1
```

```bash
DATASAFE_DATA_ROOT=./.datasafe-data scripts/vault/smoke-vault-integration.sh
```

Checks: Vault health, KV paths, `/healthz`, admin login with injected password, `security-status` with `weak_secrets=[]`.

## What runs

| Service | Profile | Role |
|---------|---------|------|
| `vault` | `vault` | Dev-mode Vault (`server -dev`, token `root`) |
| `vault-init` | `vault` | One-shot `init-kv.sh` → `secret/datasafe/*` |
| `vault-agent` | `vault` | Renders `/rendered/datasafe.env` |
| `storage-server` | (default) | `entrypoint-with-vault-env.sh` when overlay applied |

Vault API: `http://localhost:8200` (**dev only**).

## Files

| Path | Purpose |
|------|---------|
| `init-kv.sh` | KV-v2 seed (compose `vault-init`) |
| `agent.hcl` | Vault Agent config (dev token) |
| `templates/datasafe.env.tpl` | Path → `STORAGE_*` mapping |
| `entrypoint-with-vault-env.sh` | Sources rendered env (no-op if file absent and `DATASAFE_VAULT_REQUIRED` unset) |
| `test-fixtures.env` | Fixture values for CI / `test-vault-integration.mjs` |
| `local/setup-vault-dev.*` | Host bootstrap scripts |

## Production

Hardened Vault, Kubernetes auth or AppRole, and [Helm Agent Injector example](../helm/datasafe/examples/values-vault-agent.yaml). See operations guide for air-gapped notes.

## Troubleshooting: Docker proxy

On Windows, if `docker pull` or Compose fails with `connecting to 127.0.0.1:10801: connectex: ... actively refused`, WinHTTP is still pointing at a local proxy that is not running (check with `netsh winhttp show proxy`). Either start the repo workaround before pulls—`node scripts/local-direct-proxy.js` in the background, or run `scripts\ensure-docker-pull-proxy.cmd` (same idea)—or reset WinHTTP as an administrator with `netsh winhttp reset proxy` and restart Docker Desktop. Client `%USERPROFILE%\.docker\config.json` may look clean while the daemon still uses WinHTTP; fixing the dead `127.0.0.1:10801` listener is required before `hashicorp/vault:1.17` can be pulled. Prefer local images for storage-server: set `DATASAFE_SERVER_IMAGE` to a built tag and add `-f docker-compose.local-binary.yml` after `go build` into `deploy/docker/storage-server-linux`.
