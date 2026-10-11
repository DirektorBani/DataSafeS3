#!/usr/bin/env bash
# Single-node regression gate: resource budget and prod-mode bolt smoke.
# Requires Docker, curl, and python3. AWS CLI is installed when missing (smoke only).
#
#   python3 scripts/single-node-gate.py self-test
#   docker build -t datasafe-storage-server:gate -f deploy/docker/Dockerfile .
#   docker pull ghcr.io/direktorbani/datasafe-storage-server:v1.4.1
#   bash scripts/single-node-gate.sh all
#
# Core installer (after the PR image is tagged as the pin the installer writes):
#   docker tag datasafe-storage-server:gate ghcr.io/direktorbani/datasafe-storage-server:v1.4.1
#   ./install.sh --yes --profiles core --project datasafe-gate --data-root /tmp/datasafe-gate
#   bash scripts/single-node-gate.sh smoke-installed
#
# Docs: docs/operations-guide/en/single-node-gate.md
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
PY="$ROOT/scripts/single-node-gate.py"

BASE_IMAGE="${BASE_IMAGE:-ghcr.io/direktorbani/datasafe-storage-server:v1.4.1}"
PR_IMAGE="${PR_IMAGE:-datasafe-storage-server:gate}"
RUNS="${GATE_RUNS:-5}"
WARMUP="${GATE_WARMUP_SEC:-5}"
MEASURE_PORT="${GATE_MEASURE_PORT:-19000}"
SMOKE_PORT="${GATE_SMOKE_PORT:-9000}"

# Same shape as installer core: prod, bolt, compose still injects the postgres
# hostname, and no broker URL is set. Secrets are non-default except the
# first-run admin password, which the installer also leaves as admin.
GATE_ACCESS_KEY="${GATE_ACCESS_KEY:-datasafe}"
GATE_SECRET_KEY="${GATE_SECRET_KEY:-gate-secret-key-not-default-0123456789}"
GATE_JWT_SECRET="${GATE_JWT_SECRET:-gate-jwt-secret-not-default-0123456789}"
GATE_METRICS_TOKEN="${GATE_METRICS_TOKEN:-gate-metrics-token}"
GATE_ADMIN_USER="${GATE_ADMIN_USER:-admin}"
GATE_ADMIN_PASSWORD="${GATE_ADMIN_PASSWORD:-admin}"

now_ns() {
  python3 -c 'import time; print(time.perf_counter_ns())'
}

env_value() {
  local file="$1" key="$2" line=""
  line="$(grep -E "^[[:space:]]*${key}=" "$file" | tail -n 1 || true)"
  printf '%s' "${line#*=}"
}

cleanup_lone() {
  docker rm -f -v gate-measure gate-smoke >/dev/null 2>&1 || true
}
trap cleanup_lone EXIT

core_docker_run() {
  local name="$1" port="$2" image="$3"
  docker run -d --name "$name" \
    -p "127.0.0.1:${port}:9000" \
    -e STORAGE_ADDR=:9000 \
    -e STORAGE_DEV=false \
    -e STORAGE_LOG_LEVEL=info \
    -e STORAGE_DATA_DIR=/data \
    -e STORAGE_REGION=us-east-1 \
    -e STORAGE_ACCESS_KEY="$GATE_ACCESS_KEY" \
    -e STORAGE_SECRET_KEY="$GATE_SECRET_KEY" \
    -e STORAGE_ADMIN_USER="$GATE_ADMIN_USER" \
    -e STORAGE_ADMIN_PASSWORD="$GATE_ADMIN_PASSWORD" \
    -e STORAGE_JWT_SECRET="$GATE_JWT_SECRET" \
    -e STORAGE_METADATA_BACKEND=bolt \
    -e STORAGE_POSTGRES_HOST=postgres \
    -e STORAGE_POSTGRES_PORT=5432 \
    -e STORAGE_POSTGRES_USER=datasafe \
    -e STORAGE_POSTGRES_PASSWORD=datasafe \
    -e STORAGE_POSTGRES_DB=datasafe \
    -e STORAGE_METRICS_TOKEN="$GATE_METRICS_TOKEN" \
    "$image" >/dev/null
}

wait_health() {
  local url="$1" name="${2:-}"
  local deadline=$((SECONDS + 60))
  until curl -fsS "$url" >/dev/null 2>&1; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "timed out waiting for $url" >&2
      if [ -n "$name" ]; then
        docker logs "$name" >&2 || true
      fi
      return 1
    fi
    sleep 0.05
  done
}

sample_rss() {
  local name="$1"
  docker exec "$name" awk '/^VmRSS:/{print $2}' /proc/1/status
}

one_run() {
  local label="$1" image="$2"
  local name="gate-measure"
  docker rm -f -v "$name" >/dev/null 2>&1 || true
  local start_ns end_ns startup_ms
  start_ns="$(now_ns)"
  core_docker_run "$name" "$MEASURE_PORT" "$image"
  wait_health "http://127.0.0.1:${MEASURE_PORT}/healthz" "$name"
  end_ns="$(now_ns)"
  startup_ms=$(((end_ns - start_ns) / 1000000))
  local cmd
  cmd="$(docker exec "$name" cat /proc/1/cmdline | tr '\0' ' ')"
  case "$cmd" in
    *storage-server*) ;;
    *)
      echo "PID 1 is not storage-server: $cmd" >&2
      return 1
      ;;
  esac
  sleep "$WARMUP"
  local s1 s2 s3 rss
  s1="$(sample_rss "$name")"
  sleep 1
  s2="$(sample_rss "$name")"
  sleep 1
  s3="$(sample_rss "$name")"
  rss="$(python3 "$PY" median "$s1" "$s2" "$s3")"
  docker rm -f -v "$name" >/dev/null 2>&1 || true
  printf '%s\t%s\t%s\n' "$label" "$startup_ms" "$rss"
  echo "$label run startup_ms=$startup_ms rss_kib=$rss (samples $s1 $s2 $s3)" >&2
}

cmd_measure() {
  docker image inspect "$BASE_IMAGE" >/dev/null
  docker image inspect "$PR_IMAGE" >/dev/null
  local base_id pr_id
  base_id="$(docker image inspect -f '{{.Id}}' "$BASE_IMAGE")"
  pr_id="$(docker image inspect -f '{{.Id}}' "$PR_IMAGE")"
  if [ "$base_id" = "$pr_id" ]; then
    echo "baseline and PR images have the same id ($base_id)." >&2
    echo "Refusing to compare a retagged image. docker pull the v1.4.1 baseline again." >&2
    return 1
  fi
  local tsv work i
  work="$(mktemp -d)"
  tsv="$work/runs.tsv"
  : >"$tsv"
  i=1
  while [ "$i" -le "$RUNS" ]; do
    one_run baseline "$BASE_IMAGE" >>"$tsv"
    one_run pr "$PR_IMAGE" >>"$tsv"
    i=$((i + 1))
  done
  python3 - "$tsv" "$work/samples.json" "$BASE_IMAGE" "$PR_IMAGE" "$base_id" "$pr_id" <<'PY'
import json, sys
tsv, out, base_image, pr_image, base_id, pr_id = sys.argv[1:]
samples = {
    "baseline_image": base_image,
    "pr_image": pr_image,
    "baseline_id": base_id,
    "pr_id": pr_id,
    "baseline": {"startup_ms": [], "rss_kib": []},
    "pr": {"startup_ms": [], "rss_kib": []},
}
for line in open(tsv, encoding="utf-8"):
    label, startup, rss = line.split()
    samples[label]["startup_ms"].append(int(startup))
    samples[label]["rss_kib"].append(int(rss))
with open(out, "w", encoding="utf-8") as fh:
    json.dump(samples, fh)
PY
  local summary="${GITHUB_STEP_SUMMARY:-}"
  python3 "$PY" budget "$work/samples.json" --summary "$summary"
  if [ -n "${GATE_SAMPLES_OUT:-}" ]; then
    cp "$work/samples.json" "$GATE_SAMPLES_OUT"
  fi
}

ensure_aws() {
  if command -v aws >/dev/null 2>&1; then
    return 0
  fi
  if command -v sudo >/dev/null 2>&1 && command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update -y
    sudo apt-get install -y awscli
    return 0
  fi
  echo "aws CLI is required for the S3 smoke" >&2
  return 1
}

assert_core_env() {
  local name="$1"
  local dev backend pghost nats kafka
  dev="$(docker exec "$name" printenv STORAGE_DEV)"
  backend="$(docker exec "$name" printenv STORAGE_METADATA_BACKEND)"
  pghost="$(docker exec "$name" printenv STORAGE_POSTGRES_HOST)"
  nats="$(docker exec "$name" printenv STORAGE_NATS_URL 2>/dev/null || true)"
  kafka="$(docker exec "$name" printenv STORAGE_KAFKA_BROKERS 2>/dev/null || true)"
  [ "$dev" = "false" ] || { echo "STORAGE_DEV=$dev" >&2; return 1; }
  [ "$backend" = "bolt" ] || { echo "STORAGE_METADATA_BACKEND=$backend" >&2; return 1; }
  [ "$pghost" = "postgres" ] || { echo "STORAGE_POSTGRES_HOST=$pghost (core compose sets postgres)" >&2; return 1; }
  [ -z "$nats" ] || { echo "STORAGE_NATS_URL is set" >&2; return 1; }
  [ -z "$kafka" ] || { echo "STORAGE_KAFKA_BROKERS is set" >&2; return 1; }
}

assert_quiet() {
  local name="$1" api="$2" token="$3" work="$4"
  curl -fsS -H "Authorization: Bearer $token" "$api/metrics" -o "$work/metrics.txt"
  python3 "$PY" metrics-check "$work/metrics.txt"
  docker logs "$name" >"$work/server.log" 2>&1
  python3 "$PY" log-check "$work/server.log"
  {
    docker exec "$name" cat /proc/net/tcp 2>/dev/null || true
    echo
    docker exec "$name" cat /proc/net/tcp6 2>/dev/null || true
  } >"$work/net.txt"
  python3 "$PY" tcp-check "$work/net.txt"
}

smoke_body() {
  local name="$1" api="$2" s3="$3"
  local work key="hello.txt"
  work="$(mktemp -d)"
  echo "smoke api=$api s3=$s3 container=$name"
  curl -fsS "$api/healthz" >/dev/null
  bash "$ROOT/scripts/ci-complete-setup.sh" "$api"
  assert_core_env "$name"

  local tok
  tok="$(curl -fsS -X POST "$api/api/v1/admin/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${GATE_ADMIN_USER}\",\"password\":\"${GATE_ADMIN_PASSWORD}\"}" \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')"

  ensure_aws
  export AWS_ACCESS_KEY_ID="$GATE_ACCESS_KEY"
  export AWS_SECRET_ACCESS_KEY="$GATE_SECRET_KEY"
  export AWS_DEFAULT_REGION=us-east-1
  export AWS_EC2_METADATA_DISABLED=true
  export AWS_PAGER=""
  export AWS_REQUEST_CHECKSUM_CALCULATION=when_required
  export AWS_RESPONSE_CHECKSUM_VALIDATION=when_required
  export AWS_CONFIG_FILE="$work/aws-config"
  export AWS_SHARED_CREDENTIALS_FILE="$work/aws-credentials"
  cat >"$AWS_CONFIG_FILE" <<EOF
[default]
region = us-east-1
output = json
s3 =
    addressing_style = path
EOF
  cat >"$AWS_SHARED_CREDENTIALS_FILE" <<EOF
[default]
aws_access_key_id = ${GATE_ACCESS_KEY}
aws_secret_access_key = ${GATE_SECRET_KEY}
EOF
  printf 'hello-gate\n' >"$work/$key"
  local bucket="gate-smoke"
  aws s3api create-bucket --bucket "$bucket" --endpoint-url "$s3" >/dev/null
  aws s3api put-object --bucket "$bucket" --key "$key" --body "$work/$key" --endpoint-url "$s3" >/dev/null
  aws s3api get-object --bucket "$bucket" --key "$key" "$work/got.txt" --endpoint-url "$s3" >/dev/null
  cmp "$work/$key" "$work/got.txt"
  aws s3api list-objects-v2 --bucket "$bucket" --endpoint-url "$s3" --output json >"$work/list.json"
  python3 "$PY" s3-list-check "$work/list.json" "$key"

  curl -fsS -H "Authorization: Bearer $tok" "$api/api/v1/activity?limit=20&period=all" -o "$work/activity.json"
  python3 "$PY" activity-check "$work/activity.json"

  curl -fsS -X POST "$api/api/v1/inventory/jobs" \
    -H "Authorization: Bearer $tok" \
    -H 'Content-Type: application/json' \
    -d "{\"bucket\":\"${bucket}\",\"format\":\"csv\"}" \
    -o "$work/inv-create.json"
  curl -fsS -H "Authorization: Bearer $tok" "$api/api/v1/inventory/jobs" -o "$work/inv-list.json"
  python3 "$PY" inventory-check "$work/inv-create.json" "$work/inv-list.json"
  local job_id
  job_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$work/inv-create.json")"
  curl -fsS -H "Authorization: Bearer $tok" "$api/api/v1/inventory/jobs/${job_id}/download" -o "$work/inventory.csv"
  grep -q "$key" "$work/inventory.csv"

  local pack="$work/evidence-pack"
  mkdir -p "$pack"
  cp "$work/inventory.csv" "$pack/inventory-${bucket}.csv"
  curl -fsS -H "Authorization: Bearer $tok" \
    "$api/api/v1/activity/export?format=csv&period=all&bucket=${bucket}" \
    -o "$pack/activity-${bucket}.csv"
  curl -fsS -H "Authorization: Bearer $tok" \
    "$api/api/v1/activity/export?format=json&period=all&bucket=${bucket}" \
    -o "$pack/activity-${bucket}.json"
  curl -fsS -H "Authorization: Bearer $tok" \
    "$api/api/v1/buckets/${bucket}/settings" \
    -o "$pack/bucket-settings-${bucket}.json" || true
  test -s "$pack/inventory-${bucket}.csv"
  test -s "$pack/activity-${bucket}.csv"
  test -s "$pack/activity-${bucket}.json"
  printf 'gate evidence pack\nbucket=%s\n' "$bucket" >"$pack/README.txt"
  python3 - "$pack" "$work/evidence-pack.zip" <<'PY'
import sys, zipfile
from pathlib import Path
src, dest = Path(sys.argv[1]), Path(sys.argv[2])
with zipfile.ZipFile(dest, "w") as zf:
    for p in src.iterdir():
        zf.write(p, p.name)
print(f"evidence pack {dest} bytes={dest.stat().st_size}")
PY
  test -s "$work/evidence-pack.zip"

  assert_quiet "$name" "$api" "$GATE_METRICS_TOKEN" "$work"
  echo "smoke ok"
}

cmd_smoke_pr() {
  local name="gate-smoke"
  docker rm -f -v "$name" >/dev/null 2>&1 || true
  core_docker_run "$name" "$SMOKE_PORT" "$PR_IMAGE"
  wait_health "http://127.0.0.1:${SMOKE_PORT}/healthz" "$name"
  smoke_body "$name" "http://127.0.0.1:${SMOKE_PORT}" "http://127.0.0.1:${SMOKE_PORT}"
  docker rm -f -v "$name" >/dev/null 2>&1 || true
}

load_env_file() {
  local file="$1"
  GATE_ACCESS_KEY="$(env_value "$file" STORAGE_ACCESS_KEY)"
  GATE_SECRET_KEY="$(env_value "$file" STORAGE_SECRET_KEY)"
  GATE_METRICS_TOKEN="$(env_value "$file" STORAGE_METRICS_TOKEN)"
  local user pass
  user="$(env_value "$file" STORAGE_ADMIN_USER)"
  pass="$(env_value "$file" STORAGE_ADMIN_PASSWORD)"
  [ -n "$user" ] && GATE_ADMIN_USER="$user"
  [ -n "$pass" ] && GATE_ADMIN_PASSWORD="$pass"
  [ -n "$GATE_ACCESS_KEY" ] && [ -n "$GATE_SECRET_KEY" ] && [ -n "$GATE_METRICS_TOKEN" ]
}

cmd_smoke_installed() {
  local project="${GATE_PROJECT:-datasafe-gate}"
  local envfile="${GATE_ENV_FILE:-$ROOT/.env}"
  local api="${GATE_API_BASE:-http://127.0.0.1:8080}"
  local s3="${GATE_S3_URL:-http://127.0.0.1:9000}"
  load_env_file "$envfile"
  local cid
  cid="$(docker compose -p "$project" -f "$ROOT/docker-compose.yml" ps -q storage-server)"
  [ -n "$cid" ] || { echo "no storage-server in project $project" >&2; return 1; }
  wait_health "$api/healthz" "$cid"
  smoke_body "$cid" "$api" "$s3"
}

usage() {
  cat <<EOF
Usage: bash scripts/single-node-gate.sh <measure|smoke|all|smoke-installed>

  measure           interleaved idle RSS and startup vs \$BASE_IMAGE (default $BASE_IMAGE)
  smoke             run only \$PR_IMAGE (default $PR_IMAGE) and smoke it
  all               measure, then smoke
  smoke-installed   smoke the core compose project (default datasafe-gate) using .env

Env: GATE_RUNS ($RUNS), GATE_WARMUP_SEC ($WARMUP), GITHUB_STEP_SUMMARY, GATE_SAMPLES_OUT
EOF
}

main() {
  local cmd="${1:-}"
  case "$cmd" in
    measure) cmd_measure ;;
    smoke) cmd_smoke_pr ;;
    all) cmd_measure; cmd_smoke_pr ;;
    smoke-installed) cmd_smoke_installed ;;
    -h|--help|help|"") usage ;;
    *) echo "unknown command: $cmd" >&2; usage; return 1 ;;
  esac
}

main "$@"
