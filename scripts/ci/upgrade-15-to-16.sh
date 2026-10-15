#!/usr/bin/env bash
# 1.5.0 → 1.6.0 on Bolt and Postgres, plus a rollback of a volume that never
# stored an SSE-KMS object. The forward volume enables SSE-S3 and is not rolled
# back: 1.5.0 cannot read a format-1 envelope.
#
#   docker pull ghcr.io/direktorbani/datasafe-storage-server:v1.5.0
#   docker load < datasafe-storage-server-ci.tar.gz
#   bash scripts/ci/upgrade-15-to-16.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BASE_IMAGE="${BASE_IMAGE:-ghcr.io/direktorbani/datasafe-storage-server:v1.5.0}"
PR_IMAGE="${PR_IMAGE:-datasafe-storage-server:ci}"
NET="e06-upgrade"
PG="e06-upgrade-pg"
SSE_KEY="upgrade-16-sse-master-key-0123456789"

cleanup() {
  docker rm -f e06-up-bolt e06-up-pg e06-up-bolt-rb e06-up-pg-rb "$PG" >/dev/null 2>&1 || true
  docker volume rm e06-up-bolt e06-up-bolt-rb e06-up-pg e06-up-pg-rb >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_health() {
  local url="$1" name="$2"
  local deadline=$((SECONDS + 90))
  until curl -fsS "$url" >/dev/null 2>&1; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "timed out waiting for $url" >&2
      docker logs "$name" >&2 || true
      return 1
    fi
    sleep 0.5
  done
}

s3_code() {
  local method="$1" url="$2" body="${3-}"
  local out
  if [ -n "$body" ]; then
    out="$(curl --aws-sigv4 "aws:amz:us-east-1:s3" --user "datasafe:datasafesecret" \
      -sS -w '\n%{http_code}' -X "$method" \
      -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
      -H "Content-Type: application/octet-stream" \
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

start_node() {
  local name="$1" image="$2" port="$3" vol="$4" dsn="$5" ssekey="$6"
  docker rm -f "$name" >/dev/null 2>&1 || true
  local args=(
    docker run -d --name "$name" --network "$NET"
    -p "127.0.0.1:${port}:9000"
    -v "${vol}:/data"
    -e STORAGE_ADDR=:9000
    -e STORAGE_DEV=true
    -e STORAGE_LOG_LEVEL=info
    -e STORAGE_DATA_DIR=/data
    -e STORAGE_REGION=us-east-1
    -e STORAGE_ACCESS_KEY=datasafe
    -e STORAGE_SECRET_KEY=datasafesecret
    -e STORAGE_ADMIN_USER=admin
    -e STORAGE_ADMIN_PASSWORD=admin
    -e STORAGE_JWT_SECRET=upgrade-jwt-secret
    -e STORAGE_AUTO_HOME_BUCKET=false
  )
  if [ -n "$dsn" ]; then
    args+=(-e STORAGE_METADATA_BACKEND=postgres -e STORAGE_POSTGRES_DSN="$dsn")
  fi
  if [ -n "$ssekey" ]; then
    args+=(-e STORAGE_SSE_MASTER_KEY="$ssekey")
  fi
  args+=("$image")
  "${args[@]}" >/dev/null
  wait_health "http://127.0.0.1:${port}/healthz" "$name"
}

admin_token() {
  local base="$1"
  curl -sf -X POST "$base/api/v1/admin/login" \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin"}' \
    | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])'
}

prepare_api() {
  local base="$1"
  bash "$ROOT/scripts/ci-complete-setup.sh" "$base"
  local tok
  tok="$(admin_token "$base")"
  curl -sf -X POST "$base/api/v1/webhooks" \
    -H "Authorization: Bearer $tok" \
    -H 'Content-Type: application/json' \
    -d '{"name":"e06","url":"https://example.com/hook","events":["*"],"enabled":true}' >/dev/null
}

assert_webhook() {
  local base="$1"
  local tok body
  tok="$(admin_token "$base")"
  body="$(curl -sf "$base/api/v1/webhooks" -H "Authorization: Bearer $tok")"
  printf '%s' "$body" | grep -q 'https://example.com/hook'
}

forward_path() {
  local kind="$1" name="$2" vol="$3" port="$4" dsn="$5"
  echo "upgrade forward ($kind)"
  docker volume create "$vol" >/dev/null
  start_node "$name" "$BASE_IMAGE" "$port" "$vol" "$dsn" ""
  local base="http://127.0.0.1:${port}"
  prepare_api "$base"
  s3_code PUT "$base/upbox" ""
  [ "$S3_CODE" = "200" ] || { echo "create bucket $S3_CODE $S3_BODY" >&2; return 1; }
  s3_code PUT "$base/upbox/plain.txt" "plain-from-15"
  [ "$S3_CODE" = "200" ] || { echo "put 1.5 $S3_CODE $S3_BODY" >&2; return 1; }
  docker stop "$name" >/dev/null
  docker rm "$name" >/dev/null
  start_node "$name" "$PR_IMAGE" "$port" "$vol" "$dsn" "$SSE_KEY"
  s3_code GET "$base/upbox/plain.txt" ""
  [ "$S3_CODE" = "200" ] && [ "$S3_BODY" = "plain-from-15" ] || { echo "plaintext after upgrade $S3_CODE $S3_BODY" >&2; return 1; }
  local tok
  tok="$(admin_token "$base")"
  local enc
  enc="$(curl -sS -o /tmp/e06-enc-body.txt -w '%{http_code}' -X PUT "$base/api/v1/buckets/upbox/encryption" \
    -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' \
    -d '{"enabled":true}')"
  [ "$enc" = "200" ] || { echo "enable sse-s3 HTTP $enc" >&2; cat /tmp/e06-enc-body.txt >&2; return 1; }
  s3_code PUT "$base/upbox/sealed.txt" "sealed-on-16"
  [ "$S3_CODE" = "200" ] || { echo "put sse $S3_CODE $S3_BODY" >&2; return 1; }
  s3_code GET "$base/upbox/sealed.txt" ""
  [ "$S3_CODE" = "200" ] && [ "$S3_BODY" = "sealed-on-16" ] || { echo "get sse $S3_CODE $S3_BODY" >&2; return 1; }
  assert_webhook "$base"
  docker rm -f "$name" >/dev/null
  echo "upgrade forward ($kind) ok; volume is not rolled back"
}

rollback_path() {
  local kind="$1" name="$2" vol="$3" port="$4" dsn="$5"
  echo "upgrade rollback ($kind)"
  docker volume create "$vol" >/dev/null
  start_node "$name" "$BASE_IMAGE" "$port" "$vol" "$dsn" ""
  local base="http://127.0.0.1:${port}"
  bash "$ROOT/scripts/ci-complete-setup.sh" "$base"
  s3_code PUT "$base/rbbox" ""
  [ "$S3_CODE" = "200" ] || { echo "rollback create $S3_CODE $S3_BODY" >&2; return 1; }
  s3_code PUT "$base/rbbox/plain.txt" "rollback-plain"
  [ "$S3_CODE" = "200" ] || { echo "rollback put $S3_CODE $S3_BODY" >&2; return 1; }
  docker stop "$name" >/dev/null
  docker rm "$name" >/dev/null
  start_node "$name" "$PR_IMAGE" "$port" "$vol" "$dsn" ""
  s3_code GET "$base/rbbox/plain.txt" ""
  [ "$S3_CODE" = "200" ] && [ "$S3_BODY" = "rollback-plain" ] || { echo "1.6 read $S3_CODE $S3_BODY" >&2; return 1; }
  docker stop "$name" >/dev/null
  docker rm "$name" >/dev/null
  start_node "$name" "$BASE_IMAGE" "$port" "$vol" "$dsn" ""
  s3_code GET "$base/rbbox/plain.txt" ""
  [ "$S3_CODE" = "200" ] && [ "$S3_BODY" = "rollback-plain" ] || { echo "1.5 rollback read $S3_CODE $S3_BODY" >&2; return 1; }
  docker rm -f "$name" >/dev/null
  echo "upgrade rollback ($kind) ok"
}

docker network create "$NET" >/dev/null
forward_path bolt e06-up-bolt e06-up-bolt 19311 ""
rollback_path bolt e06-up-bolt-rb e06-up-bolt-rb 19312 ""

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
docker exec "$PG" psql -U datasafe -d datasafe -c 'CREATE DATABASE upfwd' >/dev/null
docker exec "$PG" psql -U datasafe -d datasafe -c 'CREATE DATABASE uprb' >/dev/null
forward_path postgres e06-up-pg e06-up-pg 19321 "postgres://datasafe:datasafe@${PG}:5432/upfwd?sslmode=disable"
rollback_path postgres e06-up-pg-rb e06-up-pg-rb 19322 "postgres://datasafe:datasafe@${PG}:5432/uprb?sslmode=disable"
echo "upgrade 1.5.0 → 1.6.0 bolt and postgres ok"
