#!/bin/sh
# Record masked fixtures from a live server.
# Usage: record.sh <1.5.0|1.6.0> <base-url>
# DATASAFE_CONTRACT_USER and DATASAFE_CONTRACT_PASSWORD default to admin.
# Tokens, passwords, signatures, and request_id are replaced with <masked>.
set -eu
version="${1:?version directory, for example 1.6.0}"
base="${2:?server base url}"
base=${base%/}
user="${DATASAFE_CONTRACT_USER:-admin}"
pass="${DATASAFE_CONTRACT_PASSWORD:-admin}"
root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
dest="$root/fixtures/$version"
mkdir -p "$dest"

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
raw = open(sys.argv[1], "r", encoding="utf-8").read()
body = json.loads(raw) if raw.strip() else {}
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
body = json.loads(raw) if raw.strip() else {}
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
  status=$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" "$base$path" "$@")
  mask_file "$tmp" > "$masked"
  write_env "$method" "$path" "$status" "$masked" "$out"
  rm -f "$tmp" "$masked"
  echo "$method $path -> $status $out"
}

capture GET /healthz "$dest/healthz.json"
capture GET /api/v1/capabilities "$dest/capabilities.json"
capture GET /api/v1/me "$dest/error.json"
login=$(mktemp)
masked=$(mktemp)
login_status=$(curl -sS -o "$login" -w '%{http_code}' -X POST "$base/api/v1/admin/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${user}\",\"password\":\"${pass}\"}")
mask_file "$login" > "$masked"
write_env POST /api/v1/admin/login "$login_status" "$masked" "$dest/login.json"
rm -f "$login" "$masked"
echo "POST /api/v1/admin/login -> $login_status $dest/login.json"
echo "fixtures in $dest are masked; review them before committing"
