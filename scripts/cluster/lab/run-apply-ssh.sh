#!/usr/bin/env bash
# Live Apply onto SSH lab nodes (requires up-ssh.sh).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
STATE="${HOME:-/tmp}/.datasafe-cluster"
INV="$STATE/inventory-lab.json"
KEYS="$ROOT/deploy/cluster/lab/keys/id_ed25519"
NODES="$STATE/lab-nodes.txt"

[ -f "$INV" ] || { echo "missing $INV — run up-ssh.sh first" >&2; exit 1; }
[ -f "$KEYS" ] || { echo "missing lab key $KEYS" >&2; exit 1; }

bash "$ROOT/scripts/cluster/cluster_apply_w2.sh" \
  --apply \
  --inventory "$INV" \
  --identity "$KEYS" \
  --lab \
  --nodes-file "$NODES" \
  --vip-s3 10.88.0.100 \
  --vip-console 10.88.0.101 \
  --vip-postgres 10.88.0.102 \
  --leader 10.88.0.10 \
  --interface eth0

echo "  [OK] live SSH Apply finished"
# Wait for storage on leader publish
i=1
while [ "$i" -le 60 ]; do
  if curl -fsS --max-time 3 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
    echo "  [OK] storage healthz :9010"
    exit 0
  fi
  sleep 2
  i=$((i + 1))
done
echo "  [!!] storage :9010 not ready (check ds-lab-node0 logs / deploy-storage)" >&2
exit 1
