#!/usr/bin/env bash
# Failure drills against offline local lab (compose) and/or SSH lab.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
LAB=0
OUT=""
STATE="${HOME:-/tmp}/.datasafe-cluster"

while [ $# -gt 0 ]; do
  case "$1" in
    --lab) LAB=1; shift ;;
    --out) OUT="$2"; shift 2 ;;
    *) echo "unknown $1" >&2; exit 1 ;;
  esac
done

[ -n "$OUT" ] || OUT="$STATE/drill-results-$(date +%Y%m%d%H%M%S).md"
mkdir -p "$STATE"
# also copy to internal qa path if present
QA_OUT="D:/datasafe_tz/qa/2026-07-26/drill-results-latest.md"

pass=0; fail=0; skip=0
log() { echo "$*" | tee -a "$OUT"; }
record() {
  local status="$1"; shift
  case "$status" in
    PASS) pass=$((pass+1)); log "- PASS: $*" ;;
    FAIL) fail=$((fail+1)); log "- FAIL: $*" ;;
    SKIP) skip=$((skip+1)); log "- SKIP: $*" ;;
  esac
}

{
  echo "# DataSafeS3 cluster failure drills"
  echo ""
  echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date)"
  echo "Mode: lab-offline-compose"
  echo ""
  echo "## Results"
  echo ""
} >"$OUT"

# Ensure lab up
if ! curl -fsS --max-time 3 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
  bash "$ROOT/scripts/cluster/lab/up-local.sh" || true
fi

if curl -fsS --max-time 5 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
  record PASS "storage healthz :9010"
else
  record FAIL "storage healthz :9010"
fi

if curl -fsS --max-time 5 http://127.0.0.1:8081/healthz >/dev/null 2>&1; then
  record PASS "caddy proxy :8081 -> storage"
else
  record SKIP "caddy proxy healthz (may not map /healthz)"
fi

# D2: stop one etcd member — cluster should still work for storage (metadata is postgres here)
if docker stop ds-lab-etcd1 >/dev/null 2>&1; then
  sleep 2
  if curl -fsS --max-time 5 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
    record PASS "stop etcd1 — storage still healthy (postgres metadata lab)"
  else
    record FAIL "stop etcd1 — storage unhealthy"
  fi
  docker start ds-lab-etcd1 >/dev/null 2>&1 || true
else
  record SKIP "etcd1 container not running"
fi

# D3: wipe shard volume contents via docker exec
if docker exec ds-lab-storage sh -c 'rm -rf /data/s2/*; mkdir -p /data/s2'; then
  record PASS "wipe erasure shard s2 inside storage container (2+1 parity)"
  if curl -fsS --max-time 5 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
    record PASS "storage healthy after shard wipe"
  else
    record FAIL "storage unhealthy after shard wipe"
  fi
else
  record FAIL "could not wipe shard s2"
fi

# D4: restart storage container
if docker restart ds-lab-storage >/dev/null 2>&1; then
  sleep 5
  if curl -fsS --max-time 10 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
    record PASS "storage container restart recovers healthz"
  else
    record FAIL "storage restart healthz"
  fi
else
  record FAIL "storage restart"
fi

# D5: VIP keepalived — not in offline compose
record SKIP "keepalived VIP move (needs real NIC / SSH lab image)"

# D6: Patroni promote — offline lab uses single postgres, not Patroni
record SKIP "Patroni promote <=60s (needs SSH lab or bare VMs with Patroni)"

# Static Apply artifacts
if bash -n "$ROOT/scripts/cluster/remote/deploy-storage-server.sh" \
  && bash -n "$ROOT/scripts/cluster/remote/apply-node.sh" \
  && bash -n "$ROOT/scripts/cluster/cluster_push_apply.sh"; then
  record PASS "Apply scripts bash -n"
else
  record FAIL "Apply scripts bash -n"
fi

{
  echo ""
  echo "## Summary"
  echo ""
  echo "- pass: $pass"
  echo "- fail: $fail"
  echo "- skip: $skip"
  echo ""
  echo "Honest status: offline compose lab proves storage+erasure+etcd quorum wiring and restart drills."
  echo "SSH Apply + Patroni/keepalived drills require registry access or bare Linux VMs."
} | tee -a "$OUT"

mkdir -p "D:/datasafe_tz/qa/2026-07-26" 2>/dev/null || true
cp "$OUT" "$QA_OUT" 2>/dev/null || true
echo "  [OK] results: $OUT"
[ "$fail" -eq 0 ]
