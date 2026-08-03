# Cluster installer — security notes (Wave 1 + Wave 2)

Threat boundary: operator workstation → SSH → Linux nodes.

## Wave 1 (inventory)

| Control | Requirement |
|---------|-------------|
| Password | PowerShell: `Read-Host -AsSecureString`. Bash: `read -rs`. Clear immediately; never in JSON/state/env/argv |
| Logging | No `Start-Transcript` / `set -x` during [P]; never echo password |
| Keys | ed25519 under `~/.datasafe-cluster/`; mode 600 on Unix; never commit |
| Host key | Interactive pin or known_hosts; abort on mismatch |
| Git | Deny private keys and inventory with secrets |

## Wave 2 (render / Apply)

| Control | Requirement |
|---------|-------------|
| Secrets file | `~/.datasafe-cluster/secrets/` only; chmod 600; never under `deploy/cluster/` or git |
| DryRun render | Redact PG / keepalived passwords (`REDACTED_*`) in PowerShell and bash renderers |
| Bash parity | `cluster_render_w2.sh` / `cluster_apply_w2.sh` — same security gate rules; no password on argv |
| NFS exports | Client list = cluster node IPs only; **never** `*` or `0.0.0.0/0` (enforced by `Test-ClusterRenderedSecurity`) |
| pg_hba | Cluster subnet / host list only — not `0.0.0.0/0` |
| sudoers | Scoped unit/exportfs/mount only — **not** `NOPASSWD:ALL` |
| keepalived | Prefer `datasafes3` + `CAP_NET_ADMIN`/`CAP_NET_RAW`; fallback root + warn (U8) |
| SSH Apply | `StrictHostKeyChecking=yes`; BatchMode for [K]; password never on argv |
| Live `-Apply` / `--apply` | Explicit flag; requires SSH keys (`--identity` if mode was P); password never on argv |
| Mode [P] bootstrap | `bootstrap_keys_p.sh`: password only via `SSH_ASKPASS` tempfile (wiped); never inventory/argv/logs |
| Remote scripts | `scripts/cluster/remote/` including `deploy-storage-server.sh`; Apply bundle chmod go-rwx; re-Apply stamps `/var/lib/datasafe/apply/last-apply.env` |
| Lab | Offline compose lab does not use lab `NOPASSWD:ALL` on real hosts; SSH lab image is disposable only |

Wave 1 does **not** perform remote Apply. Wave 2 DryRun renders configs; live push is opt-in via `cluster_apply_w2.sh --apply` or `Invoke-ClusterApplyWave2 -Apply`.
