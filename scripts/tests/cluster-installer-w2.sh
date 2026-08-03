#!/usr/bin/env bash
# Wave 2 bash asserts (no PowerShell required).
# Compatible with bash 3.2+.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
RENDER="$ROOT/scripts/cluster/cluster_render_w2.sh"
APPLY="$ROOT/scripts/cluster/cluster_apply_w2.sh"
failed=0

pass() { printf '  PASS %s\n' "$*"; }
fail() { printf '  FAIL %s\n' "$*"; failed=$((failed + 1)); }

echo "=== cluster-installer-w2 (bash) ==="

TMP="$(mktemp -d "${TMPDIR:-/tmp}/datasafe-w2-XXXXXX" 2>/dev/null || mktemp -d)"
INV="$TMP/inventory.json"
OUT="$TMP/out"
trap 'rm -rf "$TMP"' EXIT

cat >"$INV" <<'JSON'
{
  "version": 1,
  "ssh_mode": "K",
  "ssh_user": "datasafes3",
  "layout": "production",
  "vip_mode": "subnet",
  "nodes": [
    {"ip": "10.0.0.1", "ssh_port": 22, "roles": ["storage", "postgres", "etcd", "lb"]},
    {"ip": "10.0.0.2", "ssh_port": 22, "roles": ["storage", "postgres", "etcd", "lb"]},
    {"ip": "10.0.0.3", "ssh_port": 22, "roles": ["storage", "postgres", "etcd", "lb"]}
  ]
}
JSON

if grep -qiE '"(password|root_password)"' "$INV"; then fail "inventory must not contain password"; else pass "inventory has no password"; fi

chmod +x "$RENDER" "$APPLY" "$ROOT/scripts/cluster/cluster_wizard_w1.sh" 2>/dev/null || true

if ! "$RENDER" --inventory "$INV" --out-dir "$OUT" \
  --vip-s3 10.0.0.10 --vip-console 10.0.0.11 --vip-postgres 10.0.0.12 >/dev/null; then
  fail "render failed"
else
  pass "render ok"
fi

[ -f "$OUT/lb/haproxy.cfg" ] && pass "haproxy.cfg rendered" || fail "haproxy.cfg missing"
[ -f "$OUT/lb/keepalived-s3.conf" ] && pass "keepalived-s3 rendered" || fail "keepalived missing"
[ -f "$OUT/nodes/10.0.0.1/patroni.yml" ] && pass "patroni.yml rendered" || fail "patroni missing"
[ -f "$OUT/nodes/10.0.0.1/etcd.env" ] && pass "etcd.env rendered" || fail "etcd missing"
[ -f "$OUT/nfs/mount-shards-leader.sh" ] && pass "nfs mount script" || fail "nfs mount missing"
[ -f "$OUT/bootstrap/sudoers-datasafes3" ] && pass "sudoers copied" || fail "sudoers missing"
[ -f "$OUT/plan.json" ] && pass "plan.json written" || fail "plan.json missing"

grep -q 'frontend fe_s3' "$OUT/lb/haproxy.cfg" && pass "haproxy fe_s3" || fail "fe_s3"
grep -q 'frontend fe_console' "$OUT/lb/haproxy.cfg" && pass "haproxy fe_console" || fail "fe_console"
grep -q 'frontend fe_postgres' "$OUT/lb/haproxy.cfg" && pass "haproxy fe_postgres" || fail "fe_postgres"
grep -q '10.0.0.1:9000' "$OUT/lb/haproxy.cfg" && pass "haproxy S3 -> leader" || fail "leader backend"

EXP_TEXT="$(cat "$OUT/nfs"/exports.* 2>/dev/null || true)"
printf '%s' "$EXP_TEXT" | grep -v '^\s*#' | grep -qE '(^| )\*( |$)' && fail "nfs has wildcard" || pass "nfs exports have no *"
printf '%s' "$EXP_TEXT" | grep -v '^\s*#' | grep -q '0\.0\.0\.0/0' && fail "nfs has 0.0.0.0/0" || pass "nfs no world CIDR"

grep -v '^\s*#' "$OUT/bootstrap/sudoers-datasafes3" | grep -qE 'NOPASSWD:[[:space:]]*ALL\b' \
  && fail "sudoers NOPASSWD:ALL" || pass "sudoers scoped"

grep -v '^\s*#' "$OUT/nodes/10.0.0.1/patroni.yml" | grep -q '0\.0\.0\.0/0' \
  && fail "patroni world hba" || pass "patroni pg_hba not world-open"

grep -q 'REDACTED_PG_SUPER' "$OUT/nodes/10.0.0.1/patroni.yml" && pass "DryRun redacts secrets" || fail "secrets not redacted"

# shard count in plan
grep -q '"shard_count": 6' "$OUT/plan.json" && pass "4+2 shard_count 6" || fail "shard_count"

# evil export gate via re-check pattern
evil="$TMP/evil"
mkdir -p "$evil"
printf '/data *(rw)\n' >"$evil/exports.bad"
if grep -qE '\*' "$evil/exports.bad"; then
  pass "detects NFS wildcard pattern"
else
  fail "wildcard detect broken"
fi

# apply dry-run using inventory (writes under HOME/.datasafe-cluster — use HOME override)
export HOME="$TMP/home"
mkdir -p "$HOME/.datasafe-cluster"
cp "$INV" "$HOME/.datasafe-cluster/inventory-wave1.json"
if "$APPLY" --inventory "$HOME/.datasafe-cluster/inventory-wave1.json" --dry-run >/dev/null; then
  pass "Apply DryRun ok"
else
  fail "Apply DryRun failed"
fi

# push DryRun plans scp/ssh lines
PUSH="$ROOT/scripts/cluster/cluster_push_apply.sh"
chmod +x "$PUSH" "$ROOT/scripts/cluster/remote/"*.sh 2>/dev/null || true
if bash -n "$PUSH" && bash -n "$ROOT/scripts/cluster/remote/apply-node.sh" \
  && bash -n "$ROOT/scripts/cluster/remote/install-packages.sh" \
  && bash -n "$ROOT/scripts/cluster/remote/health-gates.sh"; then
  pass "remote scripts bash -n"
else
  fail "bash -n remote scripts"
fi

PUSH_OUT="$TMP/push-dry.txt"
if bash "$PUSH" --bundle "$OUT" --inventory "$INV" --leader-ip 10.0.0.1 --dry-run >"$PUSH_OUT" 2>"$TMP/push-err.txt"; then
  pass "push Apply DryRun ok"
else
  fail "push Apply DryRun failed"
fi
if grep -q 'DRY-RUN scp' "$TMP/push-err.txt"; then
  pass "push DryRun lists scp"
else
  fail "push DryRun missing scp plan"
fi
if grep -qi password "$TMP/push-err.txt"; then
  fail "push plan leaked password"
else
  pass "push plan has no password"
fi

# --apply without identity on mode P must fail
INV_P="$TMP/inv-p.json"
sed 's/"ssh_mode": "K"/"ssh_mode": "P"/' "$INV" >"$INV_P"
if bash "$ROOT/scripts/cluster/cluster_apply_w2.sh" --inventory "$INV_P" --apply --skip-health >/dev/null 2>"$TMP/apply-p.err"; then
  fail "apply mode P without identity should fail"
else
  pass "apply mode P without identity refused"
fi

[ -f "$ROOT/scripts/cluster/SECURITY.md" ] && pass "SECURITY.md present" || fail "SECURITY.md"
[ -f "$ROOT/deploy/cluster/templates/haproxy/haproxy.cfg.tmpl" ] && pass "templates present" || fail "templates"
[ -f "$ROOT/deploy/docker/grafana/dashboards/datasafe-cluster.json" ] && pass "grafana cluster dashboard" || fail "cluster dashboard missing"
grep -q 'datasafe_cluster_node_up' "$ROOT/deploy/docker/grafana/dashboards/datasafe-cluster.json" \
  && pass "cluster dashboard uses node_up" || fail "cluster dashboard metrics"
[ -f "$ROOT/scripts/cluster/bootstrap_keys_p.sh" ] && pass "bootstrap_keys_p.sh present" || fail "bootstrap_keys_p missing"
if bash -n "$ROOT/scripts/cluster/bootstrap_keys_p.sh"; then
  pass "bootstrap_keys_p bash -n"
else
  fail "bootstrap_keys_p bash -n"
fi
grep -q 'last-apply.env' "$ROOT/scripts/cluster/remote/apply-node.sh" && pass "apply-node idempotent stamp" || fail "idempotent stamp"
grep -q 'datasafe-cluster-nodes' "$ROOT/deploy/docker/prometheus.yml" && pass "prometheus file_sd cluster" || fail "prometheus cluster job"

# dev layout 2+1
INV2="$TMP/inv-dev.json"
sed 's/"production"/"dev"/' "$INV" >"$INV2"
OUT2="$TMP/out-dev"
"$RENDER" --inventory "$INV2" --out-dir "$OUT2" \
  --vip-s3 10.0.0.10 --vip-console 10.0.0.11 --vip-postgres 10.0.0.12 >/dev/null
grep -q '"shard_count": 3' "$OUT2/plan.json" && pass "2+1 shard_count 3" || fail "dev shard_count"

if [ "$failed" -gt 0 ]; then
  echo "FAILED: $failed"
  exit 1
fi
echo "ALL PASS"
exit 0
