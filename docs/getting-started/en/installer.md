English | **[Русский](../ru/installer.md)**

# Interactive installer

Quick path to a running DataSafeS3 stack: OS detect → prerequisites → numbered menu → confirm → `docker compose up` → health check.

## Run

```powershell
# Windows (PowerShell)
.\install.ps1
```

```bash
# Linux / macOS / WSL / Git Bash
chmod +x install.sh
./install.sh
```

```cmd
install.cmd
```

Non-interactive (CI / scripts):

```powershell
.\install.ps1 -Yes -Profiles core,postgres,monitoring,data
.\install.ps1 -DryRun -Yes -Profiles core,postgres,data,binary
```

```bash
./install.sh --yes --profiles core,postgres,monitoring,data
./install.sh --dry-run --yes --profiles core,postgres,data
```

In the menu:

- `5` — toggle one item  
- `1,2,3,4` or `1 5 6 7` — **set** selection (only listed items ON; core always stays on)  
- `R` — recommended (1–4)  
- `A` — all options  
- `C` — confirm · `Q` — quit

## Cluster mode (Wave 1)

At start the installer asks:

1. **Single-node** — current local Docker Compose flow (menu 1–6 below).  
2. **Cluster** — multi-VM path (≥3 Linux nodes).

Cluster Wave 1 collects inventory and SSH mode; Wave 2 **DryRun** renders Patroni/etcd, HAProxy/keepalived, and NFSv4 C1 plans (no live remote Apply unless explicit `-Apply`):

| Choice | Meaning |
|--------|---------|
| **P** (default) | Root password once later at Apply; never written to disk in Wave 1 |
| **K** | Existing SSH keys (`BatchMode` checked in live preflight) |
| Layout | **R** = 4+2 (default) · **2** = 2+1 lab |
| VIP | **V** = same-subnet floating IP · **D** = DNS / external LB |

Inventory: `%USERPROFILE%\.datasafe-cluster\inventory-wave1.json` (no secrets).  
Generated configs: `%USERPROFILE%\.datasafe-cluster\generated\` (DryRun uses redacted secrets).  
Security: [`scripts/cluster/SECURITY.md`](../../../scripts/cluster/SECURITY.md).  
Asserts: `scripts/tests/cluster-installer-w1.ps1` · `w2.ps1` (Windows) · `scripts/tests/cluster-installer-w1.sh` · `w2.sh` (bash, no pwsh required)  
DryRun: `.\install.ps1 -Cluster -DryRun -Yes` · `./install.sh --cluster --dry-run --yes`

Bash Wave 2 DryRun render: `scripts/cluster/cluster_render_w2.sh` / `cluster_apply_w2.sh`.  
Live Apply (explicit): `./scripts/cluster/cluster_apply_w2.sh --apply --inventory ~/.datasafe-cluster/inventory-wave1.json --identity ~/.ssh/id_ed25519`  
Lab (offline or SSH): `bash scripts/cluster/lab/up.sh && bash scripts/cluster/lab/run-apply.sh && bash scripts/cluster/lab/run-drills.sh --lab`  
storage-server is deployed on the leader by Apply (`deploy-storage-server.sh`) or by the offline lab compose. Failure drills run in lab; timed Patroni promote / keepalived VIP on bare metal remain explicit follow-ups — not GA multi-AZ.

## Menu → what gets installed (single-node)

| # | Id | Default | Effect |
|---|-----|---------|--------|
| 1 | `core` | on (required) | `docker-compose.yml` — storage-server + Caddy console |
| 2 | `postgres` | on | `--profile postgres`, `STORAGE_METADATA_BACKEND=postgres` |
| 3 | `monitoring` | on | Start Prometheus + Grafana (omit from `up` service list if off) |
| 4 | `data` | on | `docker-compose.local-data.yml` + create `DATASAFE_DATA_ROOT` |
| 5 | `binary` | off | `deploy/compose/docker-compose.local-binary.yml` + build Linux binary + console `dist` |
| 6 | `identity` | off | Start LDAP + Keycloak lab sidecars (`scripts/start-*-test`). Turns **dev mode on** so the HTTP lab IdP is reachable. |
| 7 | `dev` | off | `STORAGE_DEV=true` (HTTP/private outbound, skip weak-secret warnings). Also `-Dev` / `--dev`. |

SSO for the console is **OIDC** (Keycloak / other IdP via Settings), not an edge oauth2-proxy gate.

Image tag (default `v1.4.1`) sets `DATASAFE_SERVER_IMAGE` / `DATASAFE_CONSOLE_IMAGE` unless **5** (local binary) is selected.

## Dev mode and metrics (v1.4.1)

A default install sets `STORAGE_DEV=false`. Dev mode is opt-in (`-Dev`, `--dev`, or profile `dev`). The **identity** profile also sets `STORAGE_DEV=true` because the lab Keycloak issuer is plain HTTP — do not use that profile in production.

The installer generates `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY`, and `STORAGE_METRICS_TOKEN` when they are missing or still the published defaults. It does **not** change `STORAGE_ADMIN_PASSWORD` (`admin` / `admin` until the setup wizard). When monitoring is enabled, Prometheus scrapes `/metrics` with that bearer token so Grafana keeps working. Re-running the installer keeps secrets that are already non-default.

First run is unchanged: open the console, change the admin password, finish the setup wizard (external S3 is optional).

## Prerequisites

The installer checks Docker Engine + `docker compose`. If missing, it **offers** install steps (`winget` / `brew` / distro package docs) — never silently elevates or installs without confirmation. With `--yes` / `-Yes`, missing Docker is a hard error.

## After install

| Service | URL |
|---------|-----|
| Console | http://localhost:8080 (`admin` / `admin`) |
| S3 API | http://localhost:9000 |
| Grafana | http://localhost:3000 (if monitoring on) |

Next: change admin password → setup wizard → [onboarding](onboarding.md).

Overlays reference: [`deploy/compose/README.md`](../../deploy/compose/README.md).
