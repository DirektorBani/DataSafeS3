#!/usr/bin/env bash
# Full SSH lab (3 Docker "VMs"). Prefers offline image build when Docker HTTPS proxy breaks apk.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
LAB="$ROOT/deploy/cluster/lab"
KEYS="$LAB/keys"
STATE="${HOME:-/tmp}/.datasafe-cluster"
INV="$STATE/inventory-lab.json"

ok() { echo "  [OK] $*"; }
info() { echo "  $*"; }
err() { echo "  [XX] $*" >&2; exit 1; }

mkdir -p "$KEYS" "$STATE"
if [ ! -f "$KEYS/id_ed25519" ]; then
  ssh-keygen -t ed25519 -N "" -f "$KEYS/id_ed25519" -C "datasafe-lab" >/dev/null
  ok "generated lab SSH key"
fi
chmod 600 "$KEYS/id_ed25519"

# Stop offline lab containers that collide on :9010/:8081
docker rm -f ds-lab-storage ds-lab-caddy ds-lab-etcd0 ds-lab-etcd1 ds-lab-etcd2 ds-lab-postgres 2>/dev/null || true
docker rm -f ds-vm0 ds-vm1 ds-vm2 ds-leader ds-lab-node0 ds-lab-node1 ds-lab-node2 2>/dev/null || true

if ! docker image inspect datasafe-cluster-node:lab >/dev/null 2>&1; then
  info "image datasafe-cluster-node:lab missing — building offline bundle + Dockerfile.offline"
  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$ROOT/scripts/cluster/lab/fetch-offline-bundle.ps1" \
      || err "fetch-offline-bundle.ps1 failed (host must reach alpine CDN)"
  elif command -v pwsh >/dev/null 2>&1; then
    pwsh -NoProfile -File "$ROOT/scripts/cluster/lab/fetch-offline-bundle.ps1" \
      || err "fetch-offline-bundle.ps1 failed"
  else
    err "need powershell to fetch offline apk bundle"
  fi
  [ -d "$LAB/offline-apk" ] && ls "$LAB/offline-apk"/*.apk >/dev/null 2>&1 \
    || err "offline-apk empty after fetch"
  docker build -f "$LAB/Dockerfile.offline" -t datasafe-cluster-node:lab "$LAB" \
    || err "docker build Dockerfile.offline failed"
  ok "built datasafe-cluster-node:lab"
else
  ok "using existing datasafe-cluster-node:lab"
fi

cd "$LAB"
info "starting SSH lab compose..."
docker compose -p datasafe-cluster-lab -f docker-compose.yml up -d

# Containers regenerate host keys on recreate — wipe stale known_hosts for lab ports.
rm -f "$STATE/lab_known_hosts"
: >"$STATE/lab_known_hosts"
chmod 600 "$STATE/lab_known_hosts" 2>/dev/null || true

for port in 2221 2222 2223; do
  i=1
  while [ "$i" -le 90 ]; do
    if ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
      -o UserKnownHostsFile="$STATE/lab_known_hosts" \
      -i "$KEYS/id_ed25519" -p "$port" datasafes3@127.0.0.1 true 2>/dev/null; then
      ok "ssh ready :$port"
      break
    fi
    sleep 2
    i=$((i + 1))
    [ "$i" -le 90 ] || { echo "ssh not ready on $port" >&2; docker logs "ds-lab-node$((port-2221))" 2>&1 | tail -n 40; exit 1; }
  done
done

cat >"$INV" <<'JSON'
{
  "version": 1,
  "ssh_mode": "K",
  "ssh_user": "datasafes3",
  "layout": "dev",
  "vip_mode": "subnet",
  "interface": "eth0",
  "vips": {
    "s3": "10.88.0.100",
    "console": "10.88.0.101",
    "postgres": "10.88.0.102"
  },
  "nodes": [
    {"ip": "10.88.0.10", "ssh_host": "127.0.0.1", "ssh_port": 2221, "roles": ["storage", "postgres", "etcd", "lb"]},
    {"ip": "10.88.0.11", "ssh_host": "127.0.0.1", "ssh_port": 2222, "roles": ["storage", "postgres", "etcd", "lb"]},
    {"ip": "10.88.0.12", "ssh_host": "127.0.0.1", "ssh_port": 2223, "roles": ["storage", "postgres", "etcd", "lb"]}
  ],
  "lab_note": "SSH Docker VMs — live Apply + Patroni + unicast keepalived VIP"
}
JSON

cat >"$STATE/lab-nodes.txt" <<'EOF'
10.88.0.10 127.0.0.1 2221
10.88.0.11 127.0.0.1 2222
10.88.0.12 127.0.0.1 2223
EOF

echo "$LAB/docker-compose.yml" >"$STATE/lab-compose.path"
ok "inventory: $INV (vip_mode=subnet, unicast keepalived)"
echo "$INV"
