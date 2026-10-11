#!/usr/bin/env bash
# Mark first-run setup complete and leave the admin password as admin.
# The wizard requires one password change before setup/complete; this changes
# it and changes it back so e2e and feature-audit can keep signing in as admin/admin.
set -euo pipefail
base="${1:-http://127.0.0.1:8080}"

login() {
  curl -sf -X POST "$base/api/v1/admin/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"admin\",\"password\":\"$1\"}"
}

token() {
  login "$1" | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])'
}

tok="$(token admin)"
curl -sf -X POST "$base/api/v1/me/password" \
  -H "Authorization: Bearer $tok" \
  -H 'Content-Type: application/json' \
  -d '{"current_password":"admin","new_password":"admin-e2e-temp-1"}' >/dev/null
tok="$(token admin-e2e-temp-1)"
curl -sf -X POST "$base/api/v1/me/password" \
  -H "Authorization: Bearer $tok" \
  -H 'Content-Type: application/json' \
  -d '{"current_password":"admin-e2e-temp-1","new_password":"admin"}' >/dev/null
tok="$(token admin)"
curl -sf -X POST "$base/api/v1/setup/complete" \
  -H "Authorization: Bearer $tok" \
  -H 'Content-Type: application/json' \
  -d '{}' >/dev/null
echo "setup complete; admin password is admin"
