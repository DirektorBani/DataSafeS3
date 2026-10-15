#!/usr/bin/env bash
# Three storage nodes on one Postgres and one shared object volume.
# Replace 1.5.0 with 1.6.0 one node at a time. While a 1.5.0 peer is still
# answering /healthz, bucket encryption on a 1.6.0 leader is HTTP 409.
# After the last node is 1.6.0, aws:kms bucket encryption succeeds.
# Reads of plaintext objects keep working on every node (shared disk).
#
#   bash scripts/ci/rolling-upgrade.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BASE_IMAGE="${BASE_IMAGE:-ghcr.io/direktorbani/datasafe-storage-server:v1.5.0}"
PR_IMAGE="${PR_IMAGE:-datasafe-storage-server:ci}"
NET="e06-roll"
PG="e06-roll-pg"
VOL="e06-roll-data"
SSE_KEY="rolling-16-sse-master-key-0123456789"
DONE="/tmp/e06-roll-done"
OKFILE="/tmp/e06-roll-ok"
SEEN_409=0

LOAD_PID=""
cleanup() {
  touch "$DONE"
  if [ -n "${LOAD_PID:-}" ]; then
    kill "$LOAD_PID" >/dev/null 2>&1 || true
    wait "$LOAD_PID" >/dev/null 2>&1 || true
  fi
  docker rm -f e06a e06b e06c "$PG" >/dev/null 2>&1 || true
  docker volume rm "$VOL" >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -f "$DONE" "$OKFILE"
}
trap cleanup EXIT
rm -f "$DONE" "$OKFILE"

wait_health() {
  local url="$1" name="$2"
  local deadline=$((SECONDS + 120))
  until curl -fsS "$url" >/dev/null 2>&1; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "timed out waiting for $url" >&2
      docker logs "$name" >&2 || true
      return 1
    fi
    sleep 0.5
  done
}

port_of() {
  case "$1" in
    e06a) echo 19201 ;;
    e06b) echo 19202 ;;
    e06c) echo 19203 ;;
    *) echo "unknown node $1" >&2; return 1 ;;
  esac
}

health_of() {
  local port
  port="$(port_of "$1")"
  curl -fsS "http://127.0.0.1:${port}/healthz"
}

version_of() {
  health_of "$1" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("version",""))'
}

is_15() {
  case "$(version_of "$1")" in
    1.5*) return 0 ;;
    *) return 1 ;;
  esac
}

lease_holder() {
  docker exec "$PG" psql -U datasafe -d datasafe -tAc \
    "SELECT holder_id FROM ha_leader_lock WHERE lock_id='storage-server' AND expires_at > NOW()" \
    | tr -d '[:space:]'
}

start_node() {
  local name="$1" image="$2"
  local port
  port="$(port_of "$name")"
  docker rm -f "$name" >/dev/null 2>&1 || true
  local sse=()
  if [ "$image" = "$PR_IMAGE" ]; then
    sse=(-e STORAGE_SSE_MASTER_KEY="$SSE_KEY")
  fi
  docker run -d --name "$name" --network "$NET" \
    -p "127.0.0.1:${port}:9000" \
    -v "${VOL}:/data" \
    -e STORAGE_ADDR=:9000 \
    -e STORAGE_DEV=true \
    -e STORAGE_LOG_LEVEL=info \
    -e STORAGE_DATA_DIR=/data \
    -e STORAGE_REGION=us-east-1 \
    -e STORAGE_METADATA_BACKEND=postgres \
    -e "STORAGE_POSTGRES_DSN=postgres://datasafe:datasafe@${PG}:5432/datasafe?sslmode=disable" \
    -e STORAGE_HA_ENABLED=true \
    -e STORAGE_NODE_ID="$name" \
    -e STORAGE_HA_LEADER_TTL=15s \
    -e STORAGE_ADVERTISE_URL="http://${name}:9000" \
    -e STORAGE_ACCESS_KEY=datasafe \
    -e STORAGE_SECRET_KEY=datasafesecret \
    -e STORAGE_ADMIN_USER=admin \
    -e STORAGE_ADMIN_PASSWORD=admin \
    -e STORAGE_JWT_SECRET=rolling-jwt-secret \
    -e STORAGE_AUTO_HOME_BUCKET=false \
    "${sse[@]}" \
    "$image" >/dev/null
  wait_health "http://127.0.0.1:${port}/healthz" "$name"
}

s3_code() {
  local method="$1" url="$2" body="${3-}" ctype="${4:-application/octet-stream}"
  local out
  if [ -n "$body" ]; then
    out="$(curl --aws-sigv4 "aws:amz:us-east-1:s3" --user "datasafe:datasafesecret" \
      -sS -w '\n%{http_code}' -X "$method" \
      -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
      -H "Content-Type: ${ctype}" \
      --data-binary "$body" "$url" || true)"
  else
    out="$(curl --aws-sigv4 "aws:amz:us-east-1:s3" --user "datasafe:datasafesecret" \
      -sS -w '\n%{http_code}' -X "$method" \
      -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
      "$url" || true)"
  fi
  S3_CODE="$(printf '%s' "$out" | tail -n 1)"
  S3_BODY="$(printf '%s' "$out" | sed '$d')"
}

get_everywhere() {
  local path="$1" want="$2"
  local name port try ok
  for name in e06a e06b e06c; do
    port="$(port_of "$name")"
    ok=0
    for try in 1 2 3 4 5 6 7 8 9 10; do
      s3_code GET "http://127.0.0.1:${port}${path}" ""
      if [ "$S3_CODE" = "200" ] && [ "$S3_BODY" = "$want" ]; then
        ok=1
        break
      fi
      sleep 0.5
    done
    if [ "$ok" != "1" ]; then
      echo "GET $path on $name HTTP $S3_CODE body $(printf '%q' "$S3_BODY")" >&2
      return 1
    fi
  done
}

register_old_peers() {
  local nodes="["
  local first=1 name
  for name in e06a e06b e06c; do
    if is_15 "$name"; then
      if [ "$first" -eq 0 ]; then
        nodes+=","
      fi
      first=0
      nodes+="{\"id\":\"${name}\",\"address\":\"http://${name}:9000\",\"role\":\"primary\",\"status\":\"healthy\"}"
    fi
  done
  nodes+="]"
  docker exec "$PG" psql -U datasafe -d datasafe -c \
    "UPDATE system_config SET data = jsonb_set(data, '{cluster}', COALESCE(data->'cluster','{}'::jsonb) || jsonb_build_object('nodes', '${nodes}'::jsonb), true) WHERE id='system'" >/dev/null
}

clear_peers() {
  docker exec "$PG" psql -U datasafe -d datasafe -c \
    "UPDATE system_config SET data = jsonb_set(data, '{cluster,nodes}', '[]'::jsonb, true) WHERE id='system'" >/dev/null
}

encryption_xml() {
  cat <<'EOF'
<ServerSideEncryptionConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>aws:kms</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>
EOF
}

assert_mixed_409() {
  if [ "$SEEN_409" = "1" ]; then
    return 0
  fi
  local old_nodes=() name
  for name in e06a e06b e06c; do
    if is_15 "$name"; then
      old_nodes+=("$name")
    fi
  done
  if [ "${#old_nodes[@]}" -lt 1 ] || [ "${#old_nodes[@]}" -eq 3 ]; then
    return 0
  fi
  local paused=()
  for name in "${old_nodes[@]}"; do
    docker pause "$name" >/dev/null
    paused+=("$name")
  done
  local holder="" i still_old
  for i in $(seq 1 40); do
    holder="$(lease_holder || true)"
    still_old=0
    for name in "${old_nodes[@]}"; do
      if [ "$holder" = "$name" ]; then
        still_old=1
      fi
    done
    if [ -n "$holder" ] && [ "$still_old" = "0" ]; then
      break
    fi
    sleep 1
  done
  for name in "${paused[@]}"; do
    docker unpause "$name" >/dev/null
  done
  still_old=0
  for name in "${old_nodes[@]}"; do
    if [ "$holder" = "$name" ]; then
      still_old=1
    fi
  done
  if [ -z "$holder" ] || [ "$still_old" = "1" ]; then
    echo "mixed cluster did not elect a 1.6.0 leader (holder=${holder})" >&2
    return 1
  fi
  sleep 1
  register_old_peers
  local port
  port="$(port_of "$holder")"
  s3_code PUT "http://127.0.0.1:${port}/rollbox?encryption" "$(encryption_xml)" "application/xml"
  if [ "$S3_CODE" != "409" ] || ! printf '%s' "$S3_BODY" | grep -q "cluster upgrade in progress"; then
    echo "mixed encryption HTTP $S3_CODE $S3_BODY" >&2
    return 1
  fi
  SEEN_409=1
  echo "mixed cluster: aws:kms bucket encryption is 409 while a 1.5.0 peer is live"
  get_everywhere "/rollbox/a.txt" "rolling-plain"
  s3_code PUT "http://127.0.0.1:${port}/rollbox/b.txt" "during-mix"
  [ "$S3_CODE" = "200" ] || { echo "mixed put $S3_CODE $S3_BODY" >&2; return 1; }
  get_everywhere "/rollbox/b.txt" "during-mix"
}

load_loop() {
  local n=0 port
  while [ ! -f "$DONE" ]; do
    for port in 19201 19202 19203; do
      if curl --max-time 2 --aws-sigv4 "aws:amz:us-east-1:s3" --user "datasafe:datasafesecret" \
        -fsS -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
        "http://127.0.0.1:${port}/rollbox/a.txt" >/dev/null 2>&1; then
        n=$((n + 1))
        echo "$n" >"$OKFILE"
      fi
    done
    sleep 0.3
  done
}

docker network create "$NET" >/dev/null
docker volume create "$VOL" >/dev/null
docker run -d --name "$PG" --network "$NET" \
  -e POSTGRES_USER=datasafe \
  -e POSTGRES_PASSWORD=datasafe \
  -e POSTGRES_DB=datasafe \
  postgres:16 >/dev/null
for _ in $(seq 1 60); do
  if docker exec "$PG" pg_isready -U datasafe >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$PG" pg_isready -U datasafe >/dev/null

for name in e06a e06b e06c; do
  start_node "$name" "$BASE_IMAGE"
done

leader="$(lease_holder)"
for _ in $(seq 1 30); do
  leader="$(lease_holder || true)"
  [ -n "$leader" ] && break
  sleep 1
done
[ -n "$leader" ] || { echo "no leader" >&2; exit 1; }
base="http://127.0.0.1:$(port_of "$leader")"
bash "$ROOT/scripts/ci-complete-setup.sh" "$base"
s3_code PUT "$base/rollbox" ""
[ "$S3_CODE" = "200" ] || { echo "create $S3_CODE $S3_BODY" >&2; exit 1; }
s3_code PUT "$base/rollbox/a.txt" "rolling-plain"
[ "$S3_CODE" = "200" ] || { echo "put $S3_CODE $S3_BODY" >&2; exit 1; }
get_everywhere "/rollbox/a.txt" "rolling-plain"
tok="$(curl -sf -X POST "$base/api/v1/admin/login" -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"
share="$(curl -sf -X POST "$base/api/v1/buckets/rollbox/shares" \
  -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' \
  -d '{"key":"a.txt","expires_in_sec":3600}')"
share_tok="$(printf '%s' "$share" | python3 -c 'import json,sys; print(json.load(sys.stdin)["share"]["token"])')"

load_loop &
LOAD_PID=$!

for name in e06a e06b e06c; do
  echo "replace $name with 1.6.0"
  start_node "$name" "$PR_IMAGE"
  get_everywhere "/rollbox/a.txt" "rolling-plain"
  assert_mixed_409
done

if [ "$SEEN_409" != "1" ]; then
  echo "rolling upgrade never observed HTTP 409" >&2
  exit 1
fi
for name in e06a e06b e06c; do
  if is_15 "$name"; then
    echo "$name still reports 1.5 after the rolling upgrade" >&2
    exit 1
  fi
done
clear_peers
leader="$(lease_holder)"
for _ in $(seq 1 30); do
  leader="$(lease_holder || true)"
  if [ -n "$leader" ] && ! is_15 "$leader"; then
    break
  fi
  sleep 1
done
port="$(port_of "$leader")"
s3_code PUT "http://127.0.0.1:${port}/rollbox?encryption" "$(encryption_xml)" "application/xml"
if [ "$S3_CODE" != "200" ]; then
  echo "sse-kms after upgrade HTTP $S3_CODE $S3_BODY" >&2
  exit 1
fi
s3_code PUT "http://127.0.0.1:${port}/rollbox/c.txt" "after-kms"
[ "$S3_CODE" = "200" ] || { echo "put after kms $S3_CODE $S3_BODY" >&2; exit 1; }
get_everywhere "/rollbox/c.txt" "after-kms"
dl="$(curl -fsS "http://127.0.0.1:${port}/api/v1/public/share/${share_tok}/download")"
[ "$dl" = "rolling-plain" ] || { echo "share after upgrade $(printf '%q' "$dl")" >&2; exit 1; }

touch "$DONE"
wait "$LOAD_PID" || true
oks="$(cat "$OKFILE" 2>/dev/null || echo 0)"
if [ "${oks:-0}" -lt 5 ]; then
  echo "load loop recorded ${oks:-0} successful reads" >&2
  exit 1
fi
echo "rolling upgrade 1.5.0 → 1.6.0 ok (${oks} reads during the mix)"
