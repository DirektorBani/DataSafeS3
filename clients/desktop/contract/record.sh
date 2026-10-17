#!/bin/sh
# Record masked fixtures from a live server.
# Usage: record.sh <1.5.0|1.6.0> <base-url>
# DATASAFE_CONTRACT_USER and DATASAFE_CONTRACT_PASSWORD default to admin.
# Tokens, passwords, signatures, and request_id are replaced with <masked>.
# A body that is not JSON, such as Go's text/plain "404 page not found",
# is stored as body.raw. Temp files are removed on exit.
set -eu
# Git for Windows rewrites an argument that starts with /api or /healthz
# into a Windows path before python and curl see it.
if [ -n "${MSYSTEM:-}" ]; then
  export MSYS2_ARG_CONV_EXCL='*'
  export MSYS_NO_PATHCONV=1
fi
version="${1:?version directory, for example 1.6.0}"
base="${2:?server base url}"
base=${base%/}
user="${DATASAFE_CONTRACT_USER:-admin}"
pass="${DATASAFE_CONTRACT_PASSWORD:-admin}"
root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
dest="$root/fixtures/$version"
mkdir -p "$dest"
tmpfiles=""
cleanup() {
  # shellcheck disable=SC2086
  if [ -n "$tmpfiles" ]; then
    rm -f $tmpfiles
  fi
}
trap cleanup EXIT
keep() {
  tmpfiles="$tmpfiles $1"
}

mask_file() {
  src="$1"
  python3 - "$src" << 'PY'
import json, sys
secret = {"token", "password", "secret", "authorization", "signature", "request_id", "etag", "commit"}
def walk(node):
    if isinstance(node, dict):
        for key, value in list(node.items()):
            if key.lower() in secret and isinstance(value, str) and value:
                node[key] = "<masked>"
            else:
                walk(value)
    elif isinstance(node, list):
        for item in node:
            walk(item)
raw = open(sys.argv[1], "r", encoding="utf-8", errors="replace").read()
if not raw.strip():
    body = {}
else:
    try:
        body = json.loads(raw)
    except json.JSONDecodeError:
        body = {"raw": raw}
    else:
        walk(body)
json.dump(body, sys.stdout)
PY
}

write_env() {
  method="$1"
  path="$2"
  status="$3"
  bodyfile="$4"
  out="$5"
  python3 - "$method" "$path" "$status" "$bodyfile" "$out" << 'PY'
import json, pathlib, sys
method, path, status, bodyfile, out = sys.argv[1:6]
raw = pathlib.Path(bodyfile).read_text(encoding="utf-8")
if not raw.strip():
    body = {}
else:
    try:
        body = json.loads(raw)
    except json.JSONDecodeError:
        body = {"raw": raw}
doc = {"method": method, "path": path, "status": int(status), "body": body}
pathlib.Path(out).write_text(json.dumps(doc, indent=2) + "\n", encoding="utf-8")
PY
}

capture() {
  method="$1"
  path="$2"
  out="$3"
  shift 3
  tmp=$(mktemp)
  masked=$(mktemp)
  keep "$tmp"
  keep "$masked"
  status=$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" "$base$path" "$@")
  mask_file "$tmp" > "$masked"
  write_env "$method" "$path" "$status" "$masked" "$out"
  echo "$method $path -> $status $out"
}

capture GET /healthz "$dest/healthz.json"
capture GET /api/v1/capabilities "$dest/capabilities.json"
capture GET /api/v1/me "$dest/error.json"
login=$(mktemp)
masked=$(mktemp)
keep "$login"
keep "$masked"
login_status=$(curl -sS -o "$login" -w '%{http_code}' -X POST "$base/api/v1/admin/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${user}\",\"password\":\"${pass}\"}")
mask_file "$login" > "$masked"
write_env POST /api/v1/admin/login "$login_status" "$masked" "$dest/login.json"
echo "POST /api/v1/admin/login -> $login_status $dest/login.json"
echo "fixtures in $dest are masked; review them before committing"
