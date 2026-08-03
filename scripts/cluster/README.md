# Cluster installer scripts

Security model: see `SECURITY.md`.

## Modules

| File | Role |
|------|------|
| `ClusterInventory.ps1` | Parse/validate inventory (≥3 distinct non-loopback IPs) |
| `ClusterPreflight.ps1` | TCP + optional SSH BatchMode |
| `ClusterWizard.ps1` | Interactive Wave 1 → Wave 2 DryRun render |
| `ClusterPlan.ps1` | VIPs, leader, compact shard map (4+2 / 2+1) |
| `ClusterRender.ps1` | Expand `deploy/cluster/templates` + security gate |
| `ClusterApply.ps1` | DryRun plan (default) / experimental `-Apply` |
| `ClusterSsh.ps1` | ssh/scp helpers (no password on argv) |
| `cluster_wizard_w1.sh` | Interactive Wave 1 + Wave 2 DryRun (bash) |
| `cluster_render_w2.sh` | Bash render of `deploy/cluster/templates` (redacted secrets) |
| `cluster_apply_w2.sh` | Bash DryRun / `--apply` (SSH push via `cluster_push_apply.sh`) |
| `cluster_push_apply.sh` | scp bundle + remote scripts; run `apply-node.sh` per node |
| `bootstrap_keys_p.sh` | Mode [P]: root password once via ASKPASS → datasafes3 + ed25519 |
| `remote/` | Node-side package install, apply-node, health-gates |
| `../tests/cluster-installer-w1.ps1` / `.sh` | Wave 1 asserts |
| `../tests/cluster-installer-w2.ps1` / `.sh` | Wave 2 render/security/Apply DryRun asserts |

## Mode [P] then Apply

```bash
bash scripts/cluster/bootstrap_keys_p.sh --inventory ~/.datasafe-cluster/inventory-wave1.json
bash scripts/cluster/cluster_apply_w2.sh --apply --inventory ~/.datasafe-cluster/inventory-wave1.json \
  --identity ~/.ssh/datasafe_ed25519
```

## Cluster monitoring

Grafana dashboard **DataSafeS3 Cluster Status** (`deploy/docker/grafana/dashboards/datasafe-cluster.json`).
See `docs/operations-guide/en/monitoring.md`.

## Quick checks

```powershell
powershell -NoProfile -File scripts\tests\cluster-installer-w1.ps1
powershell -NoProfile -File scripts\tests\cluster-installer-w2.ps1
.\install.ps1 -Cluster -DryRun -Yes
```

```bash
bash scripts/tests/cluster-installer-w1.sh
bash scripts/tests/cluster-installer-w2.sh
./install.sh --cluster --dry-run --yes
```
