#!/bin/sh
# Live Desktop contract against one storage-server.
# DATASAFE_COMPAT_MODE=image pulls DATASAFE_COMPAT_IMAGE (GHCR tags use the v prefix).
# DATASAFE_COMPAT_MODE=dev150 builds tag v1.5.0 with no -ldflags and expects healthz version dev.
# DATASAFE_COMPAT_MODE=src161 builds origin/release/1.6.1 with Version=1.6.1.
# That tag is not in GHCR.
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"
port="${DATASAFE_COMPAT_PORT:-19150}"
base="http://127.0.0.1:${port}"
mode="${DATASAFE_COMPAT_MODE:-image}"
pid=""
work=""

cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  if [ "$mode" = "image" ]; then
    docker rm -f datasafe-compat >/dev/null 2>&1 || true
  fi
  if [ -n "$work" ]; then
    git worktree remove --force "$work" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

wait_health() {
  i=0
  while [ "$i" -lt 90 ]; do
    if curl -fsS "$base/healthz" >/dev/null 2>&1; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done
  echo "server did not answer $base/healthz" >&2
  return 1
}

server_env() {
  STORAGE_DEV=true
  STORAGE_LOG_LEVEL=info
  STORAGE_REGION=us-east-1
  STORAGE_ACCESS_KEY=datasafe
  STORAGE_SECRET_KEY=datasafesecret
  STORAGE_ADMIN_USER=admin
  STORAGE_ADMIN_PASSWORD=admin
  STORAGE_JWT_SECRET=datasafe-contract
  STORAGE_AUTO_HOME_BUCKET=false
  export STORAGE_DEV STORAGE_LOG_LEVEL STORAGE_REGION STORAGE_ACCESS_KEY STORAGE_SECRET_KEY
  export STORAGE_ADMIN_USER STORAGE_ADMIN_PASSWORD STORAGE_JWT_SECRET STORAGE_AUTO_HOME_BUCKET
}

if [ "$mode" = "dev150" ]; then
  git fetch --depth 1 origin tag v1.5.0
  work=$(mktemp -d)
  git worktree add --detach "$work" v1.5.0
  (cd "$work" && go build -o "$work/storage-server" ./cmd/storage-server)
  data=$(mktemp -d)
  server_env
  STORAGE_ADDR=":${port}" STORAGE_DATA_DIR="$data" \
    "$work/storage-server" >"$work/server.log" 2>&1 &
  pid=$!
  export DATASAFE_CONTRACT_DEV=1
elif [ "$mode" = "src161" ]; then
  git fetch --depth 1 origin release/1.6.1
  work=$(mktemp -d)
  git worktree add --detach "$work" origin/release/1.6.1
  (cd "$work" && CGO_ENABLED=0 go build \
    -ldflags "-X github.com/DirektorBani/datasafe/internal/buildinfo.Version=1.6.1" \
    -o "$work/storage-server" ./cmd/storage-server)
  data=$(mktemp -d)
  server_env
  STORAGE_ADDR=":${port}" STORAGE_DATA_DIR="$data" \
    "$work/storage-server" >"$work/server.log" 2>&1 &
  pid=$!
else
  image="${DATASAFE_COMPAT_IMAGE:?DATASAFE_COMPAT_IMAGE is required}"
  docker rm -f datasafe-compat >/dev/null 2>&1 || true
  docker pull "$image"
  docker run -d --name datasafe-compat \
    -p "127.0.0.1:${port}:9000" \
    -e STORAGE_ADDR=:9000 \
    -e STORAGE_DEV=true \
    -e STORAGE_LOG_LEVEL=info \
    -e STORAGE_DATA_DIR=/data \
    -e STORAGE_REGION=us-east-1 \
    -e STORAGE_ACCESS_KEY=datasafe \
    -e STORAGE_SECRET_KEY=datasafesecret \
    -e STORAGE_ADMIN_USER=admin \
    -e STORAGE_ADMIN_PASSWORD=admin \
    -e STORAGE_JWT_SECRET=datasafe-contract \
    -e STORAGE_AUTO_HOME_BUCKET=false \
    "$image" >/dev/null
fi

if ! wait_health; then
  if [ "$mode" = "image" ]; then
    docker logs datasafe-compat >&2 || true
  elif [ -n "$work" ]; then
    cat "$work/server.log" >&2 || true
  fi
  exit 1
fi

bash "$root/scripts/ci-complete-setup.sh" "$base"
export DATASAFE_CONTRACT_URL="$base"
go test ./clients/desktop/contract -count=1 -timeout 15m -run '^TestLive$'
