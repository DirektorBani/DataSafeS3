#!/usr/bin/env bash
# Offline lab: local images only (no registry pull / no image build).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
LAB="$ROOT/deploy/cluster/lab"
STATE="${HOME:-/tmp}/.datasafe-cluster"
mkdir -p "$STATE"

ok() { echo "  [OK] $*"; }
info() { echo "  $*"; }

IMAGE="ghcr.io/direktorbani/datasafe-storage-server:v1.1.0"
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  for c in \
    ghcr.io/direktorbani/datasafe-storage-server:v1.0.3 \
    ghcr.io/direktorbani/datasafe-storage-server:v1.0.2 \
    datasafe-storage-server:latest \
    cursor_p-storage-server:latest
  do
    if docker image inspect "$c" >/dev/null 2>&1; then
      IMAGE="$c"
      break
    fi
  done
fi
info "storage image: $IMAGE"
export DATASAFE_LAB_STORAGE_IMAGE="$IMAGE"

docker rm -f ds-vm0 ds-vm1 ds-vm2 ds-leader 2>/dev/null || true
# clean previous failed caddy
docker rm -f ds-lab-caddy ds-lab-storage 2>/dev/null || true

cd "$LAB"
docker compose -p datasafe-cluster-lab-local -f docker-compose.local.yml up -d

info "waiting for storage healthz on :9010 ..."
i=1
while [ "$i" -le 90 ]; do
  if curl -fsS --max-time 3 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
    ok "storage healthy"
    break
  fi
  sleep 2
  i=$((i + 1))
  if [ "$i" -gt 90 ]; then
    echo "  [XX] storage healthz timeout" >&2
    docker compose -p datasafe-cluster-lab-local -f docker-compose.local.yml ps
    docker logs ds-lab-storage 2>&1 | tail -n 50
    exit 1
  fi
done

cat >"$STATE/inventory-lab.json" <<'JSON'
{
  "version": 1,
  "ssh_mode": "K",
  "ssh_user": "datasafes3",
  "layout": "dev",
  "vip_mode": "dns",
  "nodes": [
    {"ip": "10.88.0.10", "ssh_host": "127.0.0.1", "ssh_port": 2221, "roles": ["storage", "postgres", "etcd", "lb"]},
    {"ip": "10.88.0.11", "ssh_host": "127.0.0.1", "ssh_port": 2222, "roles": ["storage", "postgres", "etcd", "lb"]},
    {"ip": "10.88.0.12", "ssh_host": "127.0.0.1", "ssh_port": 2223, "roles": ["storage", "postgres", "etcd", "lb"]}
  ],
  "lab_note": "offline compose lab — SSH Apply image build needs registry; runtime proof via up-local"
}
JSON

# remember compose path for down.sh
echo "$LAB/docker-compose.local.yml" >"$STATE/lab-compose.path"

ok "offline lab up"
ok "S3/API: http://127.0.0.1:9010"
ok "console proxy: http://127.0.0.1:8081"
ok "inventory: $STATE/inventory-lab.json"
cat >"$STATE/lab-nodes.txt" <<'EOF'
10.88.0.10 127.0.0.1 2221
10.88.0.11 127.0.0.1 2222
10.88.0.12 127.0.0.1 2223
EOF
ok "nodes file: $STATE/lab-nodes.txt"
echo "$STATE/inventory-lab.json"
