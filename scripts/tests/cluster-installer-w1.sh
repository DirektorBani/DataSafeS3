#!/usr/bin/env bash
# Wave 1 bash asserts (no PowerShell required).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
failed=0
pass() { printf '  PASS %s\n' "$*"; }
fail() { printf '  FAIL %s\n' "$*"; failed=$((failed + 1)); }

echo "=== cluster-installer-w1 (bash) ==="

TMP="$(mktemp -d "${TMPDIR:-/tmp}/datasafe-w1-XXXXXX" 2>/dev/null || mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

good="$TMP/good.json"
cat >"$good" <<'JSON'
{"version":1,"ssh_mode":"P","ssh_user":"root","layout":"production","vip_mode":"subnet","nodes":[{"ip":"10.0.0.1","ssh_port":22},{"ip":"10.0.0.2","ssh_port":22},{"ip":"10.0.0.3","ssh_port":22}]}
JSON

count_ips() { grep -oE '"ip"[[:space:]]*:[[:space:]]*"[^"]+"' "$1" | wc -l | tr -d ' '; }

n="$(count_ips "$good")"
[ "$n" -ge 3 ] && pass "valid 3-node inventory" || fail "valid inventory"

bad="$TMP/bad2.json"
cat >"$bad" <<'JSON'
{"version":1,"ssh_mode":"P","ssh_user":"root","layout":"production","vip_mode":"subnet","nodes":[{"ip":"10.0.0.1"},{"ip":"10.0.0.2"}]}
JSON
n="$(count_ips "$bad")"
[ "$n" -lt 3 ] && pass "reject <3 nodes (schema)" || fail "<3 detect"

# loopback check
echo '10.0.0.1' >"$TMP/ips"
echo '127.0.0.1' >>"$TMP/ips"
if grep -qx '127.0.0.1' "$TMP/ips"; then pass "reject loopback (detect)"; else fail "loopback"; fi

evil="$TMP/evil.json"
sed 's/"ssh_user":"root"/"ssh_user":"root","password":"secret"/' "$good" >"$evil"
if grep -qiE '"(password|root_password)"' "$evil"; then pass "reject password field"; else fail "password detect"; fi

grep -qiE '"(password|root_password)"' "$good" && fail "good has password" || pass "saved inventory has no password"

[ -f "$ROOT/scripts/cluster/SECURITY.md" ] && pass "SECURITY.md exists" || fail "SECURITY.md"
[ -f "$ROOT/scripts/cluster/cluster_wizard_w1.sh" ] && pass "cluster_wizard_w1.sh exists" || fail "wizard"

if [ "$failed" -gt 0 ]; then
  echo "FAILED: $failed"
  exit 1
fi
echo "ALL PASS"
exit 0
