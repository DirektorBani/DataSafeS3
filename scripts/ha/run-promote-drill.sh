#!/usr/bin/env bash
# One-command scripted-promote drill.
# Postgres primary + replica, two storage-server nodes, a webhook sink,
# object writes during promote, then scripts/collect-dr-evidence.sh.
#
#   bash scripts/ha/run-promote-drill.sh
#
# Ports (override when these are taken): HA_DRILL_HTTP_A=19100 HA_DRILL_HTTP_B=19101
# HA_DRILL_PG_A=15442 HA_DRILL_PG_B=15443 HA_DRILL_WEBHOOK=18080
# They do not use :9000 or :9001.
#
# KEEP_DRILL=1 leaves the compose project up.
# The image is deploy/docker/Dockerfile, the same build docker-build produces.
# PROMOTE_DRILL_SKIP_BUILD=1 uses an image that is already loaded
# (CI loads the docker-build artifact and sets PROMOTE_DRILL_IMAGE).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! docker info >/dev/null 2>&1; then
  if [[ "$(id -u)" -ne 0 ]]; then
    exec sudo -E bash "$0" "$@"
  fi
  echo "promote-drill: docker is not available" >&2
  exit 1
fi

export HA_DRILL_HTTP_A="${HA_DRILL_HTTP_A:-19100}"
export HA_DRILL_HTTP_B="${HA_DRILL_HTTP_B:-19101}"
export HA_DRILL_PG_A="${HA_DRILL_PG_A:-15442}"
export HA_DRILL_PG_B="${HA_DRILL_PG_B:-15443}"
export HA_DRILL_WEBHOOK="${HA_DRILL_WEBHOOK:-18080}"
export HA_DRILL_ROOT="$ROOT"
export HA_DRILL_STATE="${HA_DRILL_STATE:-/tmp/datasafe-promote-drill}"
export PROMOTE_DRILL_IMAGE="${PROMOTE_DRILL_IMAGE:-datasafe-promote-drill}"

STATE="$HA_DRILL_STATE"
mkdir -p "$STATE/webhook" "$STATE/work"
rm -f "$STATE/webhook/events.jsonl" "$STATE/ok-keys" "$STATE/stop-traffic" "$STATE/promote-timing.json"
: >"$STATE/ok-keys"

COMPOSE=(docker compose -p datasafe-promote-drill -f deploy/compose/docker-compose.promote-drill.yml)
A="http://127.0.0.1:${HA_DRILL_HTTP_A}"
B="http://127.0.0.1:${HA_DRILL_HTTP_B}"
S3="$ROOT/scripts/ha/drill_s3.py"
cleanup_done=0

dump_logs() {
  docker logs ds-promote-a >"$STATE/storage-a.log" 2>&1 || true
  docker logs ds-promote-b >"$STATE/storage-b.log" 2>&1 || true
  docker logs ds-promote-pg-a >"$STATE/postgres-a.log" 2>&1 || true
  docker logs ds-promote-pg-b >"$STATE/postgres-b.log" 2>&1 || true
  docker logs ds-promote-hook >"$STATE/webhook.log" 2>&1 || true
}

on_exit() {
  local rc=$?
  dump_logs || true
  if [[ "$cleanup_done" -eq 0 && "${KEEP_DRILL:-}" != "1" ]]; then
    "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  exit "$rc"
}
trap on_exit EXIT

port_free() {
  python3 - "$1" <<'PY'
import socket, sys
s = socket.socket()
s.settimeout(0.2)
rc = s.connect_ex(("127.0.0.1", int(sys.argv[1])))
sys.exit(0 if rc != 0 else 1)
PY
}

echo "[drill] removing a previous datasafe-promote-drill project"
"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true

for p in "$HA_DRILL_HTTP_A" "$HA_DRILL_HTTP_B" "$HA_DRILL_PG_A" "$HA_DRILL_PG_B" "$HA_DRILL_WEBHOOK"; do
  if ! port_free "$p"; then
    echo "promote-drill: 127.0.0.1:$p is in use. Set HA_DRILL_* to a free port." >&2
    exit 1
  fi
done

if [[ "${PROMOTE_DRILL_SKIP_BUILD:-}" == "1" ]]; then
  echo "[drill] using existing image $PROMOTE_DRILL_IMAGE"
  docker image inspect "$PROMOTE_DRILL_IMAGE" >/dev/null
else
  echo "[drill] building release image $PROMOTE_DRILL_IMAGE from deploy/docker/Dockerfile"
  docker build -t "$PROMOTE_DRILL_IMAGE" -f deploy/docker/Dockerfile .
fi

echo "[drill] starting postgres, webhook, and node-a"
"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
"${COMPOSE[@]}" up -d postgres-a postgres-b webhook storage-a

wait_http() {
  local url="$1" i
  for i in $(seq 1 90); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "promote-drill: timeout waiting for $url" >&2
  return 1
}

wait_http "http://127.0.0.1:${HA_DRILL_WEBHOOK}/"
wait_http "$A/healthz"

echo "[drill] waiting for node-a to hold the leader lease before node-b starts"
leader="no"
for _ in $(seq 1 40); do
  if curl -fsS --max-time 2 "$A/healthz" | grep -q '"is_leader":true'; then
    leader="yes"
    break
  fi
  sleep 1
done
if [[ "$leader" != "yes" ]]; then
  echo "promote-drill: node-a did not become leader" >&2
  curl -fsS "$A/healthz" || true
  exit 1
fi

echo "[drill] starting node-b"
"${COMPOSE[@]}" up -d storage-b
wait_http "$B/healthz"
for _ in $(seq 1 8); do
  if curl -fsS --max-time 2 "$B/healthz" | grep -q '"is_leader":true'; then
    echo "promote-drill: node-b holds the lease before promote" >&2
    exit 1
  fi
  if ! curl -fsS --max-time 2 "$A/healthz" | grep -q '"is_leader":true'; then
    echo "promote-drill: node-a lost the lease when node-b started" >&2
    exit 1
  fi
  sleep 1
done

json_get() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

echo "[drill] initial setup and webhook"
token="$(curl -fsS -X POST "$A/api/v1/admin/login" -H 'content-type: application/json' -d '{"username":"admin","password":"admin"}' | json_get token)"
setup="$(curl -fsS "$A/api/v1/setup/status" | json_get initial_setup_completed || true)"
if [[ "$setup" != "True" && "$setup" != "true" ]]; then
  curl -fsS -X POST "$A/api/v1/me/password" -H "authorization: Bearer $token" -H 'content-type: application/json' \
    -d '{"current_password":"admin","new_password":"Admin123!"}' >/dev/null
  curl -fsS -X POST "$A/api/v1/setup/complete" -H "authorization: Bearer $token" >/dev/null
  token="$(curl -fsS -X POST "$A/api/v1/admin/login" -H 'content-type: application/json' -d '{"username":"admin","password":"Admin123!"}' | json_get token)"
  curl -fsS -X POST "$A/api/v1/me/password" -H "authorization: Bearer $token" -H 'content-type: application/json' \
    -d '{"current_password":"Admin123!","new_password":"admin"}' >/dev/null
  token="$(curl -fsS -X POST "$A/api/v1/admin/login" -H 'content-type: application/json' -d '{"username":"admin","password":"admin"}' | json_get token)"
fi
curl -fsS -X POST "$A/api/v1/webhooks" -H "authorization: Bearer $token" -H 'content-type: application/json' \
  -d '{"name":"drill","url":"http://webhook:8080/hook","events":["ObjectCreated"],"enabled":true}' >/dev/null

echo x >"$STATE/work/body"
bucket_ok=0
for _ in 1 2 3 4 5 6 7 8; do
  if python3 "$S3" mb "$A" dr-drill >/dev/null; then
    bucket_ok=1
    break
  fi
  sleep 1
done
if [[ "$bucket_ok" != "1" ]]; then
  echo "promote-drill: create bucket failed" >&2
  python3 "$S3" mb "$A" dr-drill || true
  exit 1
fi

put_ok() {
  local base="$1" key="$2" try
  echo "$key" >"$STATE/work/body"
  for try in 1 2 3 4 5 6 7 8; do
    if python3 "$S3" put "$base" dr-drill "$key" "$STATE/work/body" >/dev/null; then
      echo "$key" >>"$STATE/ok-keys"
      return 0
    fi
    sleep 1
  done
  return 1
}

echo "[drill] five objects before promote"
for i in 1 2 3 4 5; do
  put_ok "$A" "obj-$i"
done

echo "[drill] waiting for replication lag 0 and the first deliveries"
for _ in $(seq 1 40); do
  lag="$(docker exec ds-promote-pg-a psql -U datasafe -d datasafe -Atqc "SELECT COALESCE((SELECT MAX(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn))::bigint FROM pg_stat_replication), -1);")"
  got="$(python3 - "$STATE/webhook/events.jsonl" <<'PY'
import json, sys
n=0
try:
    lines=open(sys.argv[1])
except FileNotFoundError:
    lines=[]
for line in lines:
    try:
        ev=json.loads(line)
    except Exception:
        continue
    if ev.get("event")=="ObjectCreated":
        n+=1
print(n)
PY
)"
  if [[ "$lag" == "0" && "$got" -ge 5 ]]; then
    break
  fi
  sleep 1
done

echo "[drill] traffic during promote, then fence and pg_promote"
(
  n=0
  while [[ ! -f "$STATE/stop-traffic" ]]; do
    n=$((n + 1))
    put_ok "$A" "during-$n" || true
    sleep 0.2
  done
) &
traffic_pid=$!

set +e
bash "$ROOT/scripts/ha/promote.sh" --execute \
  --primary ds-promote-pg-a --standby ds-promote-pg-b \
  --max-lag-bytes 1048576 \
  --timing-file "$STATE/promote-timing.json"
prom_rc=$?
set -e
touch "$STATE/stop-traffic"
wait "$traffic_pid" || true
if [[ "$prom_rc" -ne 0 ]]; then
  echo "promote-drill: promote.sh exited $prom_rc" >&2
  exit "$prom_rc"
fi

echo "[drill] point node-b at the new primary"
export STORAGE_B_DSN="postgres://datasafe:datasafe@postgres-b:5432/datasafe?sslmode=disable"
"${COMPOSE[@]}" up -d --force-recreate --no-deps storage-b

node_b_leads() {
  local hb ha
  hb="$(curl -fsS --max-time 2 "$B/healthz" || true)"
  ha="$(curl -fsS --max-time 2 "$A/healthz" || true)"
  printf '%s' "$hb" | grep -q '"is_leader":true' && ! printf '%s' "$ha" | grep -q '"is_leader":true'
}

echo "[drill] wait until node-b leads and node-a has dropped the lease"
led=0
for _ in $(seq 1 40); do
  if node_b_leads; then
    led=1
    break
  fi
  sleep 1
done
if [[ "$led" -ne 1 ]]; then
  echo "promote-drill: node-b did not become leader" >&2
  curl -fsS --max-time 2 "$B/healthz" || true
  echo
  exit 1
fi

echo "[drill] idempotent re-run (must leave node-b's lease in place)"
set +e
bash "$ROOT/scripts/ha/promote.sh" --execute \
  --primary ds-promote-pg-a --standby ds-promote-pg-b \
  --timing-file "$STATE/promote-timing-rerun.json"
rerun_rc=$?
set -e
python3 - "$STATE/promote-timing-rerun.json" "$rerun_rc" "$STATE/rerun.json" <<'PY'
import json, sys
timing = {}
try:
    timing = json.load(open(sys.argv[1], encoding="utf-8"))
except Exception:
    timing = {}
json.dump({
    "ran": True,
    "exit_code": int(sys.argv[2]),
    "skipped": timing.get("skipped") or [],
}, open(sys.argv[3], "w", encoding="utf-8"))
PY
if [[ "$rerun_rc" -ne 0 ]]; then
  echo "promote-drill: idempotent re-run exited $rerun_rc" >&2
  exit "$rerun_rc"
fi
if ! node_b_leads; then
  echo "promote-drill: idempotent re-run dropped the new leader" >&2
  exit 1
fi

echo "[drill] wait for webhook delivery of every committed key"
mapfile -t keys <"$STATE/ok-keys"
for _ in $(seq 1 60); do
  missing=0
  for key in "${keys[@]}"; do
    if ! python3 - "$key" "$STATE/webhook/events.jsonl" <<'PY'
import json, sys
key=sys.argv[1]
path=sys.argv[2]
try:
    lines=open(path)
except FileNotFoundError:
    raise SystemExit(1)
for line in lines:
    try:
        ev=json.loads(line)
    except Exception:
        continue
    data=ev.get("data")
    if isinstance(data, str):
        try:
            data=json.loads(data)
        except Exception:
            data={}
    if ev.get("event")=="ObjectCreated" and isinstance(data, dict) and data.get("key")==key:
        raise SystemExit(0)
raise SystemExit(1)
PY
    then
      missing=1
      break
    fi
  done
  if [[ "$missing" -eq 0 ]]; then
    break
  fi
  sleep 1
done
if [[ "$missing" -ne 0 ]]; then
  echo "promote-drill: webhook delivery incomplete" >&2
  exit 1
fi

echo "[drill] evidence pack"
bash "$ROOT/scripts/collect-dr-evidence.sh" \
  --out "$STATE/evidence" \
  --new-url "$B" \
  --old-url "$A" \
  --pg-new ds-promote-pg-b \
  --pg-old ds-promote-pg-a \
  --timing-file "$STATE/promote-timing.json" \
  --rerun-file "$STATE/rerun.json" \
  --keys-file "$STATE/ok-keys" \
  --webhook-log "$STATE/webhook/events.jsonl" \
  --bucket dr-drill \
  --version-file "$ROOT/VERSION"

echo "[drill] summary"
cat "$STATE/evidence/summary.md"
cleanup_done=0
