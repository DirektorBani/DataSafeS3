#!/usr/bin/env bash
# Apply path for offline lab: render + DryRun push + deploy-storage semantics via compose.
# Full SSH --apply requires scripts/cluster/lab/up.sh (registry access).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
STATE="${HOME:-/tmp}/.datasafe-cluster"
INV="$STATE/inventory-lab.json"

[ -f "$INV" ] || bash "$ROOT/scripts/cluster/lab/up-local.sh"

bash "$ROOT/scripts/cluster/cluster_apply_w2.sh" \
  --inventory "$INV" \
  --dry-run \
  --lab \
  --leader 10.88.0.10 \
  --vip-s3 10.88.0.10 \
  --vip-console 10.88.0.10 \
  --vip-postgres 10.88.0.10

# Prove storage-server deploy artifact exists and lab storage is up
bash -n "$ROOT/scripts/cluster/remote/deploy-storage-server.sh"
curl -fsS http://127.0.0.1:9010/healthz >/dev/null
echo "  [OK] lab Apply DryRun + storage healthz OK (offline mode)"
echo "  Note: live SSH Apply needs registry to build SSH node image (lab/up.sh)"
