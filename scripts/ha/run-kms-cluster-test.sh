#!/usr/bin/env bash
# Three storage nodes, Vault dev with one AppRole per node, MinIO, a webhook
# sink, and a load balancer. Ports stay off :9000 and :9001.
#
#   bash scripts/ha/run-kms-cluster-test.sh
#
# KMS_CLUSTER_SKIP_BUILD=1 uses KMS_CLUSTER_IMAGE (CI loads the docker-build
# artifact). KEEP_KMS_CLUSTER=1 leaves the project up. The host does not need
# Python: the webhook container has it, and this script is bash.
# Windows: scripts/ha/run-kms-cluster-test.ps1
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

if ! docker info >/dev/null 2>&1; then
  if [[ "$(id -u)" -ne 0 ]]; then
    exec sudo -E bash "$0" "$@"
  fi
  echo "kms-cluster: docker is not available" >&2
  exit 1
fi

export KMS_LB_PORT="${KMS_LB_PORT:-19080}"
export KMS_NODE1_PORT="${KMS_NODE1_PORT:-19111}"
export KMS_NODE2_PORT="${KMS_NODE2_PORT:-19112}"
export KMS_NODE3_PORT="${KMS_NODE3_PORT:-19113}"
export KMS_VAULT_PORT="${KMS_VAULT_PORT:-18200}"
export KMS_MINIO_PORT="${KMS_MINIO_PORT:-19090}"
export KMS_CLUSTER_IMAGE="${KMS_CLUSTER_IMAGE:-datasafe-storage-server:ci}"
case "$(uname -s 2>/dev/null || true)" in
  MINGW*|MSYS*|CYGWIN*) export MSYS_NO_PATHCONV=1 ;;
esac
STATE="${KMS_CLUSTER_STATE:-/tmp/datasafe-kms-cluster}"
export KMS_CLUSTER_ROOT="$ROOT"
export KMS_CLUSTER_STATE="$STATE"
mkdir -p "$STATE/webhook" "$STATE/evidence" "$STATE/minio" "$STATE/migrate-report" "$STATE/migrate-state"
: >"$STATE/webhook/events.jsonl"
: >"$STATE/verdicts.tsv"
FAILS=0

COMPOSE=(docker compose -p datasafe-kms-cluster -f deploy/compose/docker-compose.kms-cluster.yml)
LB="http://127.0.0.1:${KMS_LB_PORT}"
NODES=(
  "ds-kms-s1|${KMS_NODE1_PORT}"
  "ds-kms-s2|${KMS_NODE2_PORT}"
  "ds-kms-s3|${KMS_NODE3_PORT}"
)
LIVE=("ds-kms-s1" "ds-kms-s2" "ds-kms-s3")
VITE_PID=""

verdict() {
  printf '%s %s\n' "$1" "$2" >>"$STATE/verdicts.tsv"
  echo "[$2] $1"
  if [ "$2" != "PASS" ]; then
    FAILS=$((FAILS + 1))
  fi
}

on_exit() {
  local rc=$?
  if [ -n "$VITE_PID" ]; then
    kill "$VITE_PID" >/dev/null 2>&1 || true
  fi
  write_evidence || true
  if [ "${KEEP_KMS_CLUSTER:-}" != "1" ]; then
    "${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  if [ "$FAILS" -gt 0 ]; then
    exit 1
  fi
  exit "$rc"
}
trap on_exit EXIT

write_nginx() {
  {
    echo "worker_processes 1;"
    echo "events { worker_connections 128; }"
    echo "http {"
    echo "  upstream datasafe {"
    local name upstream
    for name in "${LIVE[@]}"; do
      case "$name" in
        ds-kms-s1) upstream="storage-1:9000" ;;
        ds-kms-s2) upstream="storage-2:9000" ;;
        ds-kms-s3) upstream="storage-3:9000" ;;
        *)
          echo "LB upstream has unknown node $name" >&2
          exit 1
          ;;
      esac
      echo "    server ${upstream} max_fails=1 fail_timeout=5s;"
    done
    echo "  }"
    echo "  server {"
    echo "    listen 8080;"
    echo "    client_max_body_size 0;"
    echo "    location / {"
    echo "      proxy_pass http://datasafe;"
    echo "      proxy_http_version 1.1;"
    # $host drops the port. SigV4 signs Host with the port, so the node
    # returns 403. $http_host is the header the client sent.
    echo "      proxy_set_header Host \$http_host;"
    echo "      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;"
    echo "      proxy_read_timeout 180s;"
    echo "    }"
    echo "  }"
    echo "}"
  } >"$STATE/nginx.conf"
}

# The first config is written before any node is stopped. All three backends
# must be listed, or a later SigV4 403 is easy to misread as a KMS failure.
assert_initial_lb() {
  local missing=0 node
  if ! grep -q 'proxy_set_header Host $http_host;' "$STATE/nginx.conf"; then
    echo "LB sends Host \$host, which drops the port and fails every SigV4 request" >&2
    missing=1
  fi
  for node in storage-1 storage-2 storage-3; do
    if ! grep -q "server ${node}:9000" "$STATE/nginx.conf"; then
      echo "LB upstream is missing ${node}" >&2
      missing=1
    fi
  done
  if [ "$missing" -ne 0 ]; then
    echo "---- nginx.conf ----" >&2
    cat "$STATE/nginx.conf" >&2
    exit 1
  fi
  echo "lb upstream: storage-1 storage-2 storage-3; Host=\$http_host" >&2
}

reload_lb() {
  write_nginx
  docker exec ds-kms-lb nginx -s reload >/dev/null 2>&1 || true
}

write_evidence() {
  local ev="$STATE/evidence"
  mkdir -p "$ev"
  {
    echo "# cluster-kms-gate"
    echo
    echo "| Check | Result |"
    echo "|---|---|"
    local name result
    while read -r name result; do
      [ -n "$name" ] || continue
      echo "| $name | $result |"
    done <"$STATE/verdicts.tsv"
    echo
    echo "fails=$FAILS"
  } >"$ev/summary.md"
  {
    echo '{'
    echo '  "fails": '"$FAILS"','
    echo '  "verdicts": {'
    local first=1 name result
    while read -r name result; do
      [ -n "$name" ] || continue
      if [ "$first" -eq 0 ]; then
        echo ','
      fi
      first=0
      printf '    "%s": "%s"' "$name" "$result"
    done <"$STATE/verdicts.tsv"
    echo
    echo '  }'
    echo '}'
  } >"$ev/evidence.json"
}

http_code() {
  HTTP_CODE="$(curl -sS -o "$STATE/body.txt" -w '%{http_code}' "$@" || true)"
}

json_field() {
  # encoding/json writes '&' as \u0026. A presigned URL still has to be fetched
  # with the separators the signature was computed over.
  sed -n "s/.*\"$1\":\"\\([^\"]*\\)\".*/\\1/p" "$STATE/body.txt" | head -n 1 \
    | sed -e 's/\\u0026/\&/g' -e 's/\\u003c/</g' -e 's/\\u003e/>/g'
}

token_for() {
  http_code -X POST "$LB/api/v1/admin/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"$2\"}"
  json_field token
}

api() {
  local method="$1" path="$2" tok="$3" body="${4-}"
  if [ -n "$body" ]; then
    http_code -X "$method" "$LB$path" \
      -H "Authorization: Bearer $tok" \
      -H 'Content-Type: application/json' \
      -d "$body"
  else
    http_code -X "$method" "$LB$path" \
      -H "Authorization: Bearer $tok"
  fi
}

# A signed request through the balancer that comes back as generic AccessDenied
# is the Host-header bug, not a KMS result. Stop there and print the config.
die_if_lb_sigv4() {
  local url="$1"
  case "$url" in
    "$LB"|"$LB"/*) ;;
    *) return 0 ;;
  esac
  if [ "$HTTP_CODE" != "403" ]; then
    return 0
  fi
  if ! grep -q '>access denied<' "$STATE/body.txt"; then
    return 0
  fi
  echo "SigV4 through the load balancer returned 403 AccessDenied for $url" >&2
  echo "The client signs Host with the port. nginx must proxy \$http_host, not \$host." >&2
  echo "response: $(cat "$STATE/body.txt")" >&2
  echo "---- nginx.conf ----" >&2
  cat "$STATE/nginx.conf" >&2
  verdict sigv4_host FAIL
  exit 1
}

s3_put() {
  local url="$1" data="$2"
  shift 2
  http_code --aws-sigv4 "aws:amz:us-east-1:s3" --user "datasafe:datasafesecret" \
    -X PUT -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
    "$@" --data-binary "$data" "$url"
  die_if_lb_sigv4 "$url"
}

s3_get() {
  local url="$1"
  http_code --aws-sigv4 "aws:amz:us-east-1:s3" --user "datasafe:datasafesecret" \
    -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
    "$url"
  cp "$STATE/body.txt" "$STATE/got.bin"
  die_if_lb_sigv4 "$url"
}

wait_health() {
  local url="$1" name="$2" deadline=$((SECONDS + 120))
  until curl -fsS "$url" >/dev/null 2>&1; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      echo "timed out waiting for $url" >&2
      docker logs "$name" >&2 || true
      return 1
    fi
    sleep 0.5
  done
}

count_leaders() {
  LEADER_PORT=""
  LEADER_NAME=""
  local n=0 item name port body
  for item in "${NODES[@]}"; do
    name="${item%%|*}"
    port="${item##*|}"
    local live=0
    local l
    for l in "${LIVE[@]}"; do
      if [ "$l" = "$name" ]; then
        live=1
      fi
    done
    [ "$live" -eq 1 ] || continue
    body="$(curl -fsS "http://127.0.0.1:${port}/healthz" 2>/dev/null || true)"
    if printf '%s' "$body" | grep -q '"is_leader":true'; then
      n=$((n + 1))
      LEADER_PORT="$port"
      LEADER_NAME="$name"
    fi
  done
  LEADER_COUNT=$n
}

wait_one_leader() {
  local deadline=$((SECONDS + 60))
  while [ "$SECONDS" -lt "$deadline" ]; do
    count_leaders
    if [ "$LEADER_COUNT" = "1" ]; then
      return 0
    fi
    sleep 0.5
  done
  echo "leader count is not 1" >&2
  return 1
}

drop_node() {
  local dead="$1" next=() name
  for name in "${LIVE[@]}"; do
    if [ "$name" != "$dead" ]; then
      next+=("$name")
    fi
  done
  LIVE=("${next[@]}")
  reload_lb
}

if ! docker image inspect "$KMS_CLUSTER_IMAGE" >/dev/null 2>&1; then
  if [ "${KMS_CLUSTER_SKIP_BUILD:-}" = "1" ]; then
    echo "image $KMS_CLUSTER_IMAGE is not loaded" >&2
    exit 1
  fi
  docker build -t "$KMS_CLUSTER_IMAGE" -f deploy/docker/Dockerfile .
fi

write_nginx
assert_initial_lb
"${COMPOSE[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
"${COMPOSE[@]}" up -d --force-recreate
wait_health "http://127.0.0.1:${KMS_NODE1_PORT}/healthz" ds-kms-s1
wait_health "http://127.0.0.1:${KMS_NODE2_PORT}/healthz" ds-kms-s2
wait_health "http://127.0.0.1:${KMS_NODE3_PORT}/healthz" ds-kms-s3
wait_health "http://127.0.0.1:${KMS_MINIO_PORT}/minio/health/live" ds-kms-minio || \
  wait_health "http://127.0.0.1:${KMS_MINIO_PORT}/minio/health/live" ds-kms-minio
WORK_START=$SECONDS

if wait_one_leader; then
  verdict leader PASS
else
  verdict leader FAIL
fi

TOK="$(token_for admin admin)"
api POST /api/v1/me/password "$TOK" '{"current_password":"admin","new_password":"admin-e2e-temp-1"}'
TOK="$(token_for admin admin-e2e-temp-1)"
api POST /api/v1/me/password "$TOK" '{"current_password":"admin-e2e-temp-1","new_password":"admin"}'
TOK="$(token_for admin admin)"
api POST /api/v1/setup/complete "$TOK" '{}'
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
  verdict setup PASS
else
  echo "setup $HTTP_CODE $(cat "$STATE/body.txt")" >&2
  verdict setup FAIL
fi

api POST /api/v1/webhooks "$TOK" '{"name":"kms-cluster","url":"http://hook:8080/","events":["*"],"enabled":true}'
if [ "$HTTP_CODE" = "201" ]; then
  verdict webhook PASS
else
  echo "webhook $HTTP_CODE $(cat "$STATE/body.txt")" >&2
  verdict webhook FAIL
fi

enable_kms() {
  local bucket="$1" i
  for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
    api PUT "/api/v1/buckets/${bucket}/encryption" "$TOK" '{"enabled":true,"provider":"vault-transit"}'
    if [ "$HTTP_CODE" = "200" ]; then
      return 0
    fi
    sleep 1
  done
  echo "enable $bucket $HTTP_CODE $(cat "$STATE/body.txt")" >&2
  return 1
}

s3_put "$LB/plainbox" ""
s3_put "$LB/plainbox/a.txt" "plain-body"
s3_put "$LB/kmsbox" ""
if enable_kms kmsbox; then
  s3_put "$LB/kmsbox/a.txt" "kms-body" -H "x-amz-server-side-encryption: aws:kms"
  if [ "$HTTP_CODE" = "200" ]; then
    verdict kms_put PASS
  else
    echo "kms put $HTTP_CODE $(cat "$STATE/body.txt")" >&2
    verdict kms_put FAIL
  fi
else
  verdict kms_put FAIL
fi

kms_get_ok=1
for item in "${NODES[@]}"; do
  port="${item##*|}"
  s3_get "http://127.0.0.1:${port}/kmsbox/a.txt"
  if [ "$HTTP_CODE" != "200" ] || ! cmp -s "$STATE/got.bin" <(printf '%s' 'kms-body'); then
    kms_get_ok=0
    echo "kms get :$port $HTTP_CODE" >&2
  fi
done
if [ "$kms_get_ok" -eq 1 ]; then
  verdict kms_get_all PASS
else
  verdict kms_get_all FAIL
fi

# A DEK already unwrapped stays readable while Vault is down (cache TTL).
# fresh.txt is written and never read, so its GET is a cache miss and must be 503.
s3_put "$LB/kmsbox/fresh.txt" "fresh-body" -H "x-amz-server-side-encryption: aws:kms"
if [ "$HTTP_CODE" != "200" ]; then
  echo "fresh put $HTTP_CODE $(cat "$STATE/body.txt")" >&2
  verdict vault_down FAIL
else
  docker network disconnect datasafe-kms-cluster_default ds-kms-vault || true
  sleep 2
  s3_get "$LB/kmsbox/a.txt"
  cached="$HTTP_CODE"
  s3_get "$LB/kmsbox/fresh.txt"
  kms_down="$HTTP_CODE"
  s3_get "$LB/plainbox/a.txt"
  plain_up="$HTTP_CODE"
  docker network connect datasafe-kms-cluster_default ds-kms-vault || true
  if [ "$kms_down" = "503" ] && [ "$plain_up" = "200" ]; then
    verdict vault_down PASS
  else
    echo "vault down miss=$kms_down cached=$cached plain=$plain_up" >&2
    verdict vault_down FAIL
  fi
fi
# Wait on the cache miss. A cached GET is 200 before Vault is actually back,
# and the next rotate would then be 503.
i=0
until s3_get "$LB/kmsbox/fresh.txt"; [ "$HTTP_CODE" = "200" ] && cmp -s "$STATE/got.bin" <(printf '%s' 'fresh-body'); do
  i=$((i + 1))
  if [ "$i" -gt 30 ]; then
    echo "vault back fresh=$HTTP_CODE" >&2
    verdict vault_back FAIL
    break
  fi
  sleep 1
done
if [ "$i" -le 30 ]; then
  verdict vault_back PASS
fi

s3_put "$LB/rewrapbox" ""
if enable_kms rewrapbox; then
  n=0
  while [ "$n" -lt 40 ]; do
    s3_put "$LB/rewrapbox/o$n.txt" "rewrap-$n"
    n=$((n + 1))
  done
  api GET /api/v1/buckets/rewrapbox/encryption "$TOK"
  KEY_ID="$(json_field key_id)"
  api POST "/api/v1/kms/keys/${KEY_ID}/rotate" "$TOK"
  if [ "$HTTP_CODE" != "200" ]; then
    echo "rotate $HTTP_CODE $(cat "$STATE/body.txt")" >&2
    verdict rewrap FAIL
  else
    api POST /api/v1/buckets/rewrapbox/encryption/rewrap "$TOK"
    JOB_ID="$(json_field id)"
    if [ "$HTTP_CODE" != "202" ] || [ -z "$JOB_ID" ]; then
      echo "rewrap $HTTP_CODE $(cat "$STATE/body.txt")" >&2
      verdict rewrap FAIL
    else
      docker kill "$LEADER_NAME" >/dev/null
      drop_node "$LEADER_NAME"
      if wait_one_leader; then
        deadline=$((SECONDS + 90))
        status=""
        while [ "$SECONDS" -lt "$deadline" ]; do
          api GET "/api/v1/kms/jobs/${JOB_ID}" "$TOK"
          status="$(json_field status)"
          if [ "$status" = "completed" ]; then
            break
          fi
          sleep 0.5
        done
        s3_get "$LB/rewrapbox/o0.txt"
        if [ "$status" = "completed" ] && [ "$HTTP_CODE" = "200" ] && cmp -s "$STATE/got.bin" <(printf '%s' 'rewrap-0'); then
          verdict rewrap PASS
        else
          echo "rewrap status=$status get=$HTTP_CODE" >&2
          verdict rewrap FAIL
        fi
      else
        verdict rewrap FAIL
      fi
    fi
  fi
else
  verdict rewrap FAIL
fi

# Crypto-shred the KMS bucket and require every live node to refuse within 3s.
api POST /api/v1/buckets/kmsbox/encryption/destroy "$TOK" '{"confirm":"kmsbox"}'
shred_ok=1
if [ "$HTTP_CODE" != "202" ]; then
  echo "destroy $HTTP_CODE $(cat "$STATE/body.txt")" >&2
  shred_ok=0
fi
shred_start=$(date +%s%3N 2>/dev/null || echo 0)
for item in "${NODES[@]}"; do
  name="${item%%|*}"
  port="${item##*|}"
  live=0
  for l in "${LIVE[@]}"; do
    [ "$l" = "$name" ] && live=1
  done
  [ "$live" -eq 1 ] || continue
  s3_get "http://127.0.0.1:${port}/kmsbox/a.txt"
  if [ "$HTTP_CODE" != "403" ] || ! grep -q 'Ключ шифрования уничтожен' "$STATE/body.txt"; then
    shred_ok=0
    echo "shred :$port $HTTP_CODE" >&2
  fi
done
shred_end=$(date +%s%3N 2>/dev/null || echo 0)
if [ "$shred_ok" -eq 1 ] && [ "$shred_start" != "0" ] && [ $((shred_end - shred_start)) -le 3000 ]; then
  verdict crypto_shred PASS
else
  echo "shred ms $((shred_end - shred_start)) ok=$shred_ok" >&2
  verdict crypto_shred FAIL
fi

# Sharing matrix on the surviving cluster. kmsbox is already destroyed.
s3_put "$LB/s3box" ""
s3_put "$LB/s3box/a.txt" "s3-body" -H "x-amz-server-side-encryption: AES256"
api PUT /api/v1/settings/buckets/plainbox "$TOK" '{"visibility":"public-read"}'
api PUT /api/v1/settings/buckets/s3box "$TOK" '{"visibility":"public-read"}'
http_code "$LB/plainbox/a.txt"
plain_anon="$HTTP_CODE"
http_code "$LB/s3box/a.txt"
s3_anon="$HTTP_CODE"
http_code "$LB/kmsbox/a.txt"
kms_anon="$HTTP_CODE"
api POST /api/v1/buckets/plainbox/shares "$TOK" '{"key":"a.txt","expires_in_sec":3600}'
PLAIN_TOKEN="$(json_field token)"
api POST /api/v1/presign "$TOK" '{"method":"GET","bucket":"plainbox","key":"a.txt","expires_seconds":300}'
PLAIN_URL="$(json_field url)"
api POST /api/v1/users "$TOK" '{"username":"stranger","password":"stranger-pass-1","role":"user"}'
STRANGER="$(token_for stranger stranger-pass-1)"
api POST /api/v1/buckets/plainbox/shares "$STRANGER" '{"key":"a.txt","expires_in_sec":60}'
stranger_code="$HTTP_CODE"
api PUT /api/v1/buckets/plainbox/sharing "$TOK" '{"share_links_allowed":false}'
api POST /api/v1/buckets/plainbox/shares "$TOK" '{"key":"a.txt","expires_in_sec":60}'
off_share="$HTTP_CODE"
api POST /api/v1/presign "$TOK" '{"method":"GET","bucket":"plainbox","key":"a.txt","expires_seconds":60}'
off_pre="$HTTP_CODE"
api PUT /api/v1/buckets/plainbox/sharing "$TOK" '{"share_links_allowed":true}'
http_code "$LB/api/v1/public/share/${PLAIN_TOKEN}/download"
share_code="$HTTP_CODE"
http_code "$PLAIN_URL"
pre_code="$HTTP_CODE"
if [ "$plain_anon" = "200" ] && [ "$s3_anon" = "200" ] && [ "$kms_anon" = "403" ] \
  && [ "$share_code" = "200" ] && [ "$pre_code" = "200" ] \
  && [ "$stranger_code" = "403" ] && [ "$off_share" = "403" ] && [ "$off_pre" = "403" ]; then
  verdict share_matrix PASS
else
  echo "matrix anon=$plain_anon/$s3_anon/$kms_anon share=$share_code pre=$pre_code stranger=$stranger_code off=$off_share/$off_pre" >&2
  verdict share_matrix FAIL
fi

deadline=$((SECONDS + 30))
while [ "$SECONDS" -lt "$deadline" ]; do
  if grep -q 'ShareCreated' "$STATE/webhook/events.jsonl" && grep -q 'PresignIssued' "$STATE/webhook/events.jsonl"; then
    break
  fi
  sleep 0.5
done
if grep -q 'ShareCreated' "$STATE/webhook/events.jsonl" && grep -q 'PresignIssued' "$STATE/webhook/events.jsonl" \
  && ! grep -q 'wrapped_dek' "$STATE/webhook/events.jsonl"; then
  verdict events PASS
else
  echo "webhook events missing or contain a DEK" >&2
  verdict events FAIL
fi

# MinIO copy through the balancer. Kill the leader during the copy, then resume.
http_code --aws-sigv4 "aws:amz:us-east-1:s3" --user "minioadmin:minioadmin" \
  -X PUT -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
  "http://127.0.0.1:${KMS_MINIO_PORT}/migsrc"
dd if=/dev/urandom bs=1024 count=512 of="$STATE/minio/blob.bin" status=none
http_code --aws-sigv4 "aws:amz:us-east-1:s3" --user "minioadmin:minioadmin" \
  -X PUT -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
  --data-binary @"$STATE/minio/blob.bin" \
  "http://127.0.0.1:${KMS_MINIO_PORT}/migsrc/blob.bin"
go build -o "$STATE/storage-cli" ./cmd/storage-cli
targets=()
for item in "${NODES[@]}"; do
  name="${item%%|*}"
  port="${item##*|}"
  for l in "${LIVE[@]}"; do
    if [ "$l" = "$name" ]; then
      targets+=(--target "http://127.0.0.1:${port}")
    fi
  done
done
(
  "$STATE/storage-cli" migrate from-minio \
    --source "http://127.0.0.1:${KMS_MINIO_PORT}" \
    --source-key minioadmin --source-secret minioadmin \
    --target "$LB" "${targets[@]}" \
    --target-key datasafe --target-secret datasafesecret \
    --buckets migsrc --concurrency 1 \
    --state "$STATE/migrate-state" --report "$STATE/migrate-report"
) >"$STATE/migrate-first.log" 2>&1 &
mig_pid=$!
sleep 0.4
if [ -n "${LEADER_NAME:-}" ]; then
  docker kill "$LEADER_NAME" >/dev/null 2>&1 || true
  drop_node "$LEADER_NAME" || true
  wait_one_leader || true
fi
wait "$mig_pid" || true
"$STATE/storage-cli" migrate from-minio \
  --source "http://127.0.0.1:${KMS_MINIO_PORT}" \
  --source-key minioadmin --source-secret minioadmin \
  --target "$LB" \
  --target-key datasafe --target-secret datasafesecret \
  --buckets migsrc --concurrency 1 \
  --state "$STATE/migrate-state" --report "$STATE/migrate-report" \
  >"$STATE/migrate-second.log" 2>&1 || true
if [ -f "$STATE/migrate-report/evidence.json" ] && grep -q '"PASS"' "$STATE/migrate-report/evidence.json" \
  && ! grep -q '"FAIL"' "$STATE/migrate-report/evidence.json"; then
  verdict migrate PASS
else
  echo "migrate evidence missing or FAIL" >&2
  cat "$STATE/migrate-second.log" >&2 || true
  verdict migrate FAIL
fi

# kms-settings.spec.ts expects a console where Vault is not configured.
# This gate's nodes already have AppRole in the environment, so that spec
# stays on the single-node e2e job. These two specs are the encryption UI.
if [ "${KMS_CLUSTER_PLAYWRIGHT:-1}" = "1" ] && command -v npm >/dev/null 2>&1; then
  install_start=$SECONDS
  (cd web/console && npm ci && npx playwright install chromium --with-deps)
  WORK_START=$((WORK_START + SECONDS - install_start))
  (cd web/console && DS_API_PROXY="$LB" npm run dev -- --host 127.0.0.1 --port 15173) >"$STATE/vite.log" 2>&1 &
  VITE_PID=$!
  vite_deadline=$((SECONDS + 60))
  until curl -fsS http://127.0.0.1:15173/ | grep -q 'id="root"'; do
    if [ "$SECONDS" -ge "$vite_deadline" ]; then
      echo "console did not start" >&2
      cat "$STATE/vite.log" >&2 || true
      verdict playwright FAIL
      break
    fi
    sleep 0.5
  done
  if [ "$FAILS" -eq 0 ] || ! grep -q '^playwright FAIL$' "$STATE/verdicts.tsv"; then
    if (cd web/console && PLAYWRIGHT_BASE_URL=http://127.0.0.1:15173 npx playwright test --workers=1 \
      e2e/encryption-toggle.spec.ts e2e/encryption-jobs.spec.ts); then
      verdict playwright PASS
    else
      verdict playwright FAIL
    fi
  fi
else
  verdict playwright FAIL
fi

elapsed=$((SECONDS - WORK_START))
echo "scenario seconds $elapsed"
if [ "$elapsed" -lt 900 ]; then
  verdict duration PASS
else
  echo "scenario exceeded 15 minutes ($elapsed s)" >&2
  verdict duration FAIL
fi

cp "$STATE/migrate-report/evidence.json" "$STATE/evidence/migrate-evidence.json" 2>/dev/null || true
if [ "$FAILS" -eq 0 ]; then
  echo "cluster-kms-gate ok"
else
  echo "cluster-kms-gate failed ($FAILS)" >&2
  exit 1
fi
