#!/bin/bash
# Compare GET / (aws s3 ls and mc ls) for console keys on the published v1.5.0
# image and this build. A bucket owner, a bucket-admin grant, an operator, and
# an administrator must list the same bucket names on both. An owner whose key
# is also named by a read policy on an unrelated bucket must still list the
# owned buckets (this build may also list that policy bucket). On this build,
# a key that owns nothing, has no grant, and has no s3:ListAllMyBuckets allow
# is denied, including when a GetObject policy names it.
set -euo pipefail

BASELINE_IMAGE="${BASELINE_IMAGE:-ghcr.io/direktorbani/datasafe-storage-server:v1.5.0}"
HEAD_IMAGE="${HEAD_IMAGE:-datasafe-storage-server:ci}"
BASE_PORT="${BASE_PORT:-19150}"
HEAD_PORT="${HEAD_PORT:-19151}"
BASE_NAME="${BASE_NAME:-datasafe-list-base}"
HEAD_NAME="${HEAD_NAME:-datasafe-list-head}"

if ! command -v aws >/dev/null || ! command -v mc >/dev/null || ! command -v docker >/dev/null; then
  echo "aws, mc, and docker are required" >&2
  exit 1
fi

cleanup() {
  docker rm -f "$BASE_NAME" "$HEAD_NAME" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

start_one() {
  local name="$1" image="$2" port="$3"
  docker run -d --name "$name" -p "${port}:9000" \
    -e STORAGE_DEV=true \
    -e STORAGE_AUTO_HOME_BUCKET=false \
    -e STORAGE_ADDR=:9000 \
    -e STORAGE_DATA_DIR=/data \
    -e STORAGE_REGION=us-east-1 \
    -e STORAGE_ACCESS_KEY=datasafe \
    -e STORAGE_SECRET_KEY=datasafesecret \
    -e STORAGE_ADMIN_USER=admin \
    -e STORAGE_ADMIN_PASSWORD=admin \
    -e STORAGE_JWT_SECRET=datasafe-jwt-secret \
    -e STORAGE_METADATA_BACKEND=bolt \
    "$image" >/dev/null
  local i
  for i in $(seq 1 60); do
    if curl -sf "http://127.0.0.1:${port}/healthz" >/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "server $name did not become healthy" >&2
  docker logs "$name" >&2 || true
  return 1
}

json_field() {
  python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

api() {
  local method="$1" url="$2" token="${3:-}" body="${4:-}"
  local out code
  out="$(mktemp)"
  if [ -n "$token" ]; then
    code="$(curl -sS -o "$out" -w '%{http_code}' -X "$method" "$url" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${token}" \
      -d "$body")"
  else
    code="$(curl -sS -o "$out" -w '%{http_code}' -X "$method" "$url" \
      -H 'Content-Type: application/json' \
      -d "$body")"
  fi
  if [ "$code" -lt 200 ] || [ "$code" -ge 300 ]; then
    echo "HTTP $code $method $url" >&2
    cat "$out" >&2
    rm -f "$out"
    exit 1
  fi
  cat "$out"
  rm -f "$out"
}

# Prints access_key and secret_key for owner, grantee, operator, and admin.
provision() {
  local base="$1"
  local admin owner_tok grantee_id
  admin="$(api POST "$base/api/v1/admin/login" "" '{"username":"admin","password":"admin"}' | json_field token)"
  api POST "$base/api/v1/me/password" "$admin" '{"current_password":"admin","new_password":"Adminpass1"}' >/dev/null
  api POST "$base/api/v1/setup/complete" "$admin" '{}' >/dev/null

  local grantee_json
  api POST "$base/api/v1/users" "$admin" '{"username":"owner","password":"Ownerpass1","role":"user","email":"owner@test.com"}' >/dev/null
  grantee_json="$(api POST "$base/api/v1/users" "$admin" '{"username":"grantee","password":"Granteepass1","role":"user","email":"grantee@test.com"}')"
  grantee_id="$(printf '%s' "$grantee_json" | json_field id)"
  api POST "$base/api/v1/users" "$admin" '{"username":"ops","password":"Operatorpass1","role":"operator","email":"ops@test.com"}' >/dev/null

  owner_tok="$(api POST "$base/api/v1/admin/login" "" '{"username":"owner","password":"Ownerpass1"}' | json_field token)"
  api POST "$base/api/v1/buckets/own-bucket" "$owner_tok" '{}' >/dev/null
  local owner_key
  owner_key="$(api POST "$base/api/v1/keys" "$owner_tok" '{"label":"owner"}')"

  api POST "$base/api/v1/buckets/shared-bucket" "$admin" '{}' >/dev/null
  api PUT "$base/api/v1/buckets/shared-bucket/access" "$admin" "{\"grants\":[{\"user_id\":\"${grantee_id}\",\"level\":\"admin\",\"can_read\":true,\"can_write\":true}]}" >/dev/null

  local grantee_tok op_tok
  grantee_tok="$(api POST "$base/api/v1/admin/login" "" '{"username":"grantee","password":"Granteepass1"}' | json_field token)"
  op_tok="$(api POST "$base/api/v1/admin/login" "" '{"username":"ops","password":"Operatorpass1"}' | json_field token)"
  local grantee_key op_key admin_key
  grantee_key="$(api POST "$base/api/v1/keys" "$grantee_tok" '{"label":"grantee"}')"
  op_key="$(api POST "$base/api/v1/keys" "$op_tok" '{"label":"ops"}')"
  admin_key="$(api POST "$base/api/v1/keys" "$admin" '{"label":"admin"}')"

  python3 - "$owner_key" "$grantee_key" "$op_key" "$admin_key" <<'PY'
import json, sys
roles = ("owner", "grantee", "operator", "admin")
for role, raw in zip(roles, sys.argv[1:]):
    doc = json.loads(raw)
    print(f"{role} {doc['access_key']} {doc['secret_key']}")
PY
}

names_from_aws() {
  awk 'NF { print $NF }' | sed 's:/*$::' | sort
}

names_from_mc() {
  awk 'NF { print $NF }' | sed 's:/*$::' | sort
}

list_aws() {
  local url="$1" key="$2" secret="$3"
  AWS_ACCESS_KEY_ID="$key" AWS_SECRET_ACCESS_KEY="$secret" AWS_DEFAULT_REGION=us-east-1 \
    AWS_EC2_METADATA_DISABLED=true \
    AWS_REQUEST_CHECKSUM_CALCULATION=when_required \
    AWS_RESPONSE_CHECKSUM_VALIDATION=when_required \
    aws s3 ls --endpoint-url "$url"
}

list_mc() {
  local alias="$1" url="$2" key="$3" secret="$4"
  mc alias set "$alias" "$url" "$key" "$secret" >/dev/null
  mc ls "$alias"
}

compare_role() {
  local role="$1" bkey="$2" bsec="$3" hkey="$4" hsec="$5"
  local b_aws h_aws b_mc h_mc
  b_aws="$(list_aws "http://127.0.0.1:${BASE_PORT}" "$bkey" "$bsec" | names_from_aws)"
  h_aws="$(list_aws "http://127.0.0.1:${HEAD_PORT}" "$hkey" "$hsec" | names_from_aws)"
  b_mc="$(list_mc "base-${role}" "http://127.0.0.1:${BASE_PORT}" "$bkey" "$bsec" | names_from_mc)"
  h_mc="$(list_mc "head-${role}" "http://127.0.0.1:${HEAD_PORT}" "$hkey" "$hsec" | names_from_mc)"
  if [ "$b_aws" != "$h_aws" ] || [ "$b_mc" != "$h_mc" ] || [ "$h_aws" != "$h_mc" ]; then
    echo "list mismatch for $role" >&2
    echo "baseline aws: [$b_aws]" >&2
    echo "head aws: [$h_aws]" >&2
    echo "baseline mc: [$b_mc]" >&2
    echo "head mc: [$h_mc]" >&2
    exit 1
  fi
  case "$role" in
    owner) printf '%s\n' "$h_aws" | grep -qx 'own-bucket' ;;
    grantee) printf '%s\n' "$h_aws" | grep -qx 'shared-bucket' ;;
    operator|admin)
      printf '%s\n' "$h_aws" | grep -qx 'own-bucket'
      printf '%s\n' "$h_aws" | grep -qx 'shared-bucket'
      ;;
  esac
  echo "$role aws/mc $(echo "$h_aws" | tr '\n' ' ')"
}

echo "starting $BASELINE_IMAGE and $HEAD_IMAGE"
start_one "$BASE_NAME" "$BASELINE_IMAGE" "$BASE_PORT"
start_one "$HEAD_NAME" "$HEAD_IMAGE" "$HEAD_PORT"

base_keys="$(mktemp)"
head_keys="$(mktemp)"
provision "http://127.0.0.1:${BASE_PORT}" >"$base_keys"
provision "http://127.0.0.1:${HEAD_PORT}" >"$head_keys"

while read -r role bkey bsec; do
  hline="$(awk -v r="$role" '$1==r { print $2, $3 }' "$head_keys")"
  hkey="${hline%% *}"
  hsec="${hline#* }"
  compare_role "$role" "$bkey" "$bsec" "$hkey" "$hsec"
done <"$base_keys"

field_of() {
  local file="$1" role="$2" n="$3"
  awk -v r="$role" -v n="$n" '$1==r { print $n }' "$file"
}

# A read policy on an unrelated bucket must not turn the owner's list into 403.
# This build may also return that policy bucket. v1.5.0 must stay successful,
# and every name it returns must still be present here.
put_other_read() {
  local port="$1" owner_key="$2"
  local admin policy
  admin="$(api POST "http://127.0.0.1:${port}/api/v1/admin/login" "" '{"username":"admin","password":"Adminpass1"}' | json_field token)"
  api POST "http://127.0.0.1:${port}/api/v1/buckets/other" "$admin" '{}' >/dev/null
  policy="$(printf '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::datasafe:access-key/%s"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::other/*"}]}' "$owner_key")"
  api PUT "http://127.0.0.1:${port}/api/v1/buckets/other/policy" "$admin" "$(python3 -c 'import json,sys; print(json.dumps({"policy": sys.argv[1]}))' "$policy")" >/dev/null
}

names_or_fail() {
  local label="$1" kind="$2"
  shift 2
  local out err code
  out="$(mktemp)"
  err="$(mktemp)"
  set +e
  if [ "$kind" = aws ]; then
    list_aws "$@" >"$out" 2>"$err"
  else
    list_mc "$@" >"$out" 2>"$err"
  fi
  code=$?
  set -e
  if [ "$code" -ne 0 ]; then
    echo "$label failed ($code)" >&2
    cat "$err" >&2
    rm -f "$out" "$err"
    exit 1
  fi
  if [ "$kind" = aws ]; then
    names_from_aws <"$out"
  else
    names_from_mc <"$out"
  fi
  rm -f "$out" "$err"
}

require_subset() {
  local label="$1" base="$2" head="$3"
  local missing
  missing="$(comm -23 <(printf '%s\n' "$base" | sed '/^$/d') <(printf '%s\n' "$head" | sed '/^$/d') || true)"
  if [ -n "$missing" ]; then
    echo "$label missing names that v1.5.0 returned:" >&2
    printf '%s\n' "$missing" >&2
    echo "head was:" >&2
    printf '%s\n' "$head" >&2
    exit 1
  fi
}

base_owner_key="$(field_of "$base_keys" owner 2)"
base_owner_sec="$(field_of "$base_keys" owner 3)"
head_owner_key="$(field_of "$head_keys" owner 2)"
head_owner_sec="$(field_of "$head_keys" owner 3)"
put_other_read "$BASE_PORT" "$base_owner_key"
put_other_read "$HEAD_PORT" "$head_owner_key"

base_aws="$(names_or_fail "baseline owner after other" aws "http://127.0.0.1:${BASE_PORT}" "$base_owner_key" "$base_owner_sec")"
head_aws="$(names_or_fail "head owner after other" aws "http://127.0.0.1:${HEAD_PORT}" "$head_owner_key" "$head_owner_sec")"
base_mc="$(names_or_fail "baseline owner mc after other" mc "base-owner-other" "http://127.0.0.1:${BASE_PORT}" "$base_owner_key" "$base_owner_sec")"
head_mc="$(names_or_fail "head owner mc after other" mc "head-owner-other" "http://127.0.0.1:${HEAD_PORT}" "$head_owner_key" "$head_owner_sec")"
if ! printf '%s\n' "$head_aws" | grep -qx 'own-bucket'; then
  echo "head aws owner list dropped own-bucket: [$head_aws]" >&2
  exit 1
fi
if ! printf '%s\n' "$head_mc" | grep -qx 'own-bucket'; then
  echo "head mc owner list dropped own-bucket: [$head_mc]" >&2
  exit 1
fi
require_subset "head aws" "$base_aws" "$head_aws"
require_subset "head mc" "$base_mc" "$head_mc"
echo "owner named on other still lists own-bucket"

# Nothing owned or granted, and no ListAllMyBuckets allow: this build denies
# the list. v1.5.0 may return an empty 200, so the two sides are not compared.
head_admin="$(api POST "http://127.0.0.1:${HEAD_PORT}/api/v1/admin/login" "" '{"username":"admin","password":"Adminpass1"}' | json_field token)"
api POST "http://127.0.0.1:${HEAD_PORT}/api/v1/users" "$head_admin" '{"username":"empty","password":"Emptypass1","role":"user","email":"empty@test.com"}' >/dev/null
empty_tok="$(api POST "http://127.0.0.1:${HEAD_PORT}/api/v1/admin/login" "" '{"username":"empty","password":"Emptypass1"}' | json_field token)"
empty_raw="$(api POST "http://127.0.0.1:${HEAD_PORT}/api/v1/keys" "$empty_tok" '{"label":"empty"}')"
empty_key="$(printf '%s' "$empty_raw" | json_field access_key)"
empty_sec="$(printf '%s' "$empty_raw" | json_field secret_key)"

api POST "http://127.0.0.1:${HEAD_PORT}/api/v1/users" "$head_admin" '{"username":"bound","password":"Boundpass1","role":"user","email":"bound@test.com"}' >/dev/null
bound_tok="$(api POST "http://127.0.0.1:${HEAD_PORT}/api/v1/admin/login" "" '{"username":"bound","password":"Boundpass1"}' | json_field token)"
bound_raw="$(api POST "http://127.0.0.1:${HEAD_PORT}/api/v1/keys" "$bound_tok" '{"label":"bound"}')"
bound_key="$(printf '%s' "$bound_raw" | json_field access_key)"
bound_sec="$(printf '%s' "$bound_raw" | json_field secret_key)"
policy="$(printf '{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::datasafe:access-key/%s"},"Action":"s3:GetObject","Resource":"arn:aws:s3:::own-bucket/*"}]}' "$bound_key")"
api PUT "http://127.0.0.1:${HEAD_PORT}/api/v1/buckets/own-bucket/policy" "$head_admin" "$(python3 -c 'import json,sys; print(json.dumps({"policy": sys.argv[1]}))' "$policy")" >/dev/null

expect_denied() {
  local label="$1" key="$2" secret="$3" alias="$4"
  local aws_code mc_code
  set +e
  list_aws "http://127.0.0.1:${HEAD_PORT}" "$key" "$secret" >/dev/null 2>"$(mktemp)"
  aws_code=$?
  list_mc "$alias" "http://127.0.0.1:${HEAD_PORT}" "$key" "$secret" >/dev/null 2>"$(mktemp)"
  mc_code=$?
  set -e
  if [ "$aws_code" -eq 0 ] || [ "$mc_code" -eq 0 ]; then
    echo "$label was listed (aws $aws_code mc $mc_code)" >&2
    exit 1
  fi
  echo "$label aws exit $aws_code mc exit $mc_code"
}

expect_denied "empty key" "$empty_key" "$empty_sec" "head-empty"
expect_denied "get-only key" "$bound_key" "$bound_sec" "head-bound"
echo "list-buckets clients match v1.5.0"
