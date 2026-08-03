#!/usr/bin/env bash
# Deploy storage-server on cluster leader (Docker). Run as root on leader.
# SECURITY: uses existing DATASAFE_* env / secrets file; no password echo.
set -euo pipefail

LEADER_IP=""
BUNDLE=""
IMAGE="${DATASAFE_SERVER_IMAGE:-ghcr.io/direktorbani/datasafe-storage-server:v1.2.0}"
DATA_ROOT="/var/lib/datasafe"
PG_VIP=""
LAYOUT="production"

err() { echo "[XX] $*" >&2; exit 1; }
ok()  { echo "[OK] $*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --leader-ip) LEADER_IP="$2"; shift 2 ;;
    --bundle) BUNDLE="$2"; shift 2 ;;
    --image) IMAGE="$2"; shift 2 ;;
    --pg-vip) PG_VIP="$2"; shift 2 ;;
    --layout) LAYOUT="$2"; shift 2 ;;
    *) err "unknown $1" ;;
  esac
done

[ "$(id -u)" -eq 0 ] || err "must run as root"
[ -n "$LEADER_IP" ] || LEADER_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
[ -n "$LEADER_IP" ] || err "leader ip required"

if [ -n "$BUNDLE" ] && [ -f "$BUNDLE/plan.json" ]; then
  LAYOUT="$(sed -n 's/.*"layout"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$BUNDLE/plan.json" | head -n1)"
  [ -n "$LAYOUT" ] || LAYOUT="production"
  PG_VIP="$(sed -n 's/.*"postgres"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$BUNDLE/plan.json" | head -n1)"
fi

# Build erasure paths list from mounted shards
PATHS=""
for d in "$DATA_ROOT"/erasure/shard*; do
  [ -d "$d" ] || continue
  [ -n "$PATHS" ] && PATHS="${PATHS},"
  PATHS="${PATHS}${d}"
done
if [ -z "$PATHS" ]; then
  # local lab fallback: create 3 or 6 local shard dirs
  need=6
  [ "$LAYOUT" = "dev" ] && need=3
  i=0
  while [ "$i" -lt "$need" ]; do
    mkdir -p "$DATA_ROOT/erasure/shard$i"
    [ -n "$PATHS" ] && PATHS="${PATHS},"
    PATHS="${PATHS}$DATA_ROOT/erasure/shard$i"
    i=$((i + 1))
  done
fi

PG_HOST="${PG_VIP:-127.0.0.1}"
# In SSH lab, Patroni listens on fabric NODE_IP (not 127.0.0.1) to avoid VIP/HAProxy bind clashes.
if [ -f /etc/datasafe/lab-mode ]; then
  PG_HOST="$LEADER_IP"
fi
SECRETS="/etc/datasafe/cluster-secrets.env"
if [ -f "$SECRETS" ]; then
  # shellcheck disable=SC1090
  set -a; . "$SECRETS"; set +a
fi
PG_PASS="${PG_SUPER_PASSWORD:-datasafesuper}"
JWT="${STORAGE_JWT_SECRET:-datasafe-lab-jwt-secret-change-me}"
S3_KEY="${STORAGE_ACCESS_KEY:-datasafe}"
S3_SEC="${STORAGE_SECRET_KEY:-datasafesecret}"

if ! command -v docker >/dev/null 2>&1; then
  err "docker CLI missing on leader (lab mounts docker.sock; prod install docker engine)"
fi

# stop previous lab container if any
docker rm -f datasafe-storage-leader 2>/dev/null || true

# Lab SSH "VM": share network stack with the node container so :9000 maps via compose publish.
# sudo drops compose ENV — derive container name from hostname (node0 -> ds-lab-node0).
NET_ARGS=(--network host)
if [ -f /etc/datasafe/lab-mode ]; then
  lab_net="${DATASAFE_LAB_NET_CONTAINER:-}"
  if [ -z "$lab_net" ]; then
    hn="$(hostname -s 2>/dev/null || hostname | cut -d. -f1)"
    lab_net="ds-lab-${hn}"
  fi
  NET_ARGS=(--network "container:${lab_net}")
  IMAGE="${DATASAFE_SERVER_IMAGE:-ghcr.io/direktorbani/datasafe-storage-server:v1.1.0}"
fi

docker run -d --name datasafe-storage-leader --restart unless-stopped \
  "${NET_ARGS[@]}" \
  -e STORAGE_ADDR="${LEADER_IP}:9000" \
  -e STORAGE_DATA_DIR="$DATA_ROOT/objects" \
  -e STORAGE_METADATA_BACKEND=postgres \
  -e STORAGE_POSTGRES_DSN="postgres://postgres:${PG_PASS}@${PG_HOST}:5432/postgres?sslmode=disable" \
  -e STORAGE_OBJECT_BACKEND=erasure \
  -e STORAGE_ERASURE_LAYOUT="$LAYOUT" \
  -e STORAGE_ERASURE_DATA_PATHS="$PATHS" \
  -e STORAGE_ACCESS_KEY="$S3_KEY" \
  -e STORAGE_SECRET_KEY="$S3_SEC" \
  -e STORAGE_ADMIN_USER=admin \
  -e STORAGE_ADMIN_PASSWORD=admin \
  -e STORAGE_JWT_SECRET="$JWT" \
  -e STORAGE_REGION=us-east-1 \
  -e STORAGE_DEV=true \
  -v "$DATA_ROOT:$DATA_ROOT" \
  "$IMAGE" \
  || err "failed to start storage-server container"

# wait health — storage binds LEADER_IP:9000 (lab/colocated VIP HAProxy); not 127.0.0.1
health_url="http://${LEADER_IP}:9000/healthz"
i=1
while [ "$i" -le 60 ]; do
  if curl -fsS "$health_url" >/dev/null 2>&1; then
    ok "storage-server healthy on $health_url (erasure paths=$PATHS)"
    exit 0
  fi
  sleep 2
  i=$((i + 1))
done
err "storage-server healthz timeout ($health_url)"
