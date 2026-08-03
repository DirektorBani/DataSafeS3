#!/usr/bin/env bash
# Failure drills for SSH Docker lab — NO SKIP for VIP / Patroni (release gate).
# Requires: up-ssh.sh + run-apply-ssh.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
STATE="${HOME:-/tmp}/.datasafe-cluster"
KEYS="$ROOT/deploy/cluster/lab/keys/id_ed25519"
OUT="${1:-$STATE/drill-results-ssh-$(date +%Y%m%d%H%M%S).md}"
QA_OUT="D:/datasafe_tz/qa/2026-07-26/drill-results-ssh-latest.md"

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

ssh_n() {
  local port="$1"; shift
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile="$STATE/lab_known_hosts" \
    -i "$KEYS" -p "$port" datasafes3@127.0.0.1 "sudo $*"
}

{
  echo "# DataSafeS3 cluster failure drills (SSH Docker VMs)"
  echo ""
  echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date)"
  echo "Mode: ssh-docker-vms"
  echo ""
  echo "## Results"
  echo ""
} >"$OUT"

[ -f "$KEYS" ] || { record FAIL "lab SSH key missing"; echo "fail=$fail"; exit 1; }

# SSH ports
for port in 2221 2222 2223; do
  if ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile="$STATE/lab_known_hosts" \
    -o ConnectTimeout=5 \
    -i "$KEYS" -p "$port" datasafes3@127.0.0.1 true 2>/dev/null; then
    record PASS "ssh datasafes3@127.0.0.1:$port"
  else
    record FAIL "ssh datasafes3@127.0.0.1:$port"
  fi
done

if curl -fsS --max-time 5 http://127.0.0.1:9010/healthz >/dev/null 2>&1; then
  record PASS "storage healthz :9010 (leader publish)"
else
  record FAIL "storage healthz :9010"
fi

# Patroni: list members via REST on each node
pat_ok=0
for port in 8008 8009 8010; do
  if curl -fsS --max-time 3 "http://127.0.0.1:${port}/patroni" >/dev/null 2>&1 \
    || curl -fsS --max-time 3 "http://127.0.0.1:${port}/primary" >/dev/null 2>&1; then
    pat_ok=$((pat_ok + 1))
  fi
done
if [ "$pat_ok" -ge 1 ]; then
  record PASS "Patroni REST reachable on $pat_ok/3 published ports"
else
  # fallback: inside containers
  if docker exec ds-lab-node0 curl -sf http://127.0.0.1:8008/patroni >/dev/null 2>&1; then
    record PASS "Patroni REST inside node0"
  else
    record FAIL "Patroni REST not reachable"
  fi
fi

# D6: Patroni promote / failover <= 60s
PRIMARY_BEFORE="$(docker exec ds-lab-node0 curl -sf http://127.0.0.1:8008/patroni 2>/dev/null | sed -n 's/.*\"role\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | head -n1 || true)"
# Find current leader container
leader_c=""
for c in ds-lab-node0 ds-lab-node1 ds-lab-node2; do
  role="$(docker exec "$c" curl -sf http://127.0.0.1:8008/patroni 2>/dev/null | tr ',' '\n' | sed -n 's/.*\"role\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p' | head -n1 || true)"
  if [ "$role" = "master" ] || [ "$role" = "primary" ] || [ "$role" = "leader" ]; then
    leader_c="$c"
    break
  fi
done
if [ -z "$leader_c" ]; then
  # patroni JSON may use "state":"running" + role master
  for c in ds-lab-node0 ds-lab-node1 ds-lab-node2; do
    if docker exec "$c" curl -sf http://127.0.0.1:8008/primary >/dev/null 2>&1; then
      leader_c="$c"
      break
    fi
  done
fi

if [ -n "$leader_c" ]; then
  t0="$(date +%s)"
  docker stop "$leader_c" >/dev/null
  promoted=0
  for _ in $(seq 1 60); do
    for c in ds-lab-node0 ds-lab-node1 ds-lab-node2; do
      [ "$c" = "$leader_c" ] && continue
      if docker exec "$c" curl -sf http://127.0.0.1:8008/primary >/dev/null 2>&1; then
        promoted=1
        break
      fi
    done
    [ "$promoted" = "1" ] && break
    sleep 1
  done
  t1="$(date +%s)"
  elapsed=$((t1 - t0))
  docker start "$leader_c" >/dev/null || true
  sleep 5
  if [ "$promoted" = "1" ] && [ "$elapsed" -le 60 ]; then
    record PASS "Patroni promote/failover in ${elapsed}s (killed $leader_c)"
  elif [ "$promoted" = "1" ]; then
    record FAIL "Patroni promote took ${elapsed}s (>60)"
  else
    record FAIL "Patroni promote did not elect new primary within 60s"
  fi
else
  record FAIL "could not detect Patroni primary before failover drill"
fi

# D5: keepalived VIP move (unicast on labnet)
VIP="10.88.0.100"
owner=""
for c in ds-lab-node0 ds-lab-node1 ds-lab-node2; do
  if docker exec "$c" ip -o addr show 2>/dev/null | grep -q " ${VIP}/"; then
    owner="$c"
    break
  fi
done
if [ -z "$owner" ]; then
  # wait a bit for VRRP
  sleep 5
  for c in ds-lab-node0 ds-lab-node1 ds-lab-node2; do
    if docker exec "$c" ip -o addr show 2>/dev/null | grep -q " ${VIP}/"; then
      owner="$c"
      break
    fi
  done
fi

if [ -z "$owner" ]; then
  record FAIL "VIP $VIP not present on any node (keepalived/unicast)"
else
  record PASS "VIP $VIP owned by $owner"
  docker exec "$owner" sh -c 'killall keepalived 2>/dev/null || pkill keepalived || true'
  moved=0
  for _ in $(seq 1 30); do
    for c in ds-lab-node0 ds-lab-node1 ds-lab-node2; do
      [ "$c" = "$owner" ] && continue
      if docker exec "$c" ip -o addr show 2>/dev/null | grep -q " ${VIP}/"; then
        moved=1
        record PASS "VIP $VIP moved to $c after stop keepalived on $owner"
        break
      fi
    done
    [ "$moved" = "1" ] && break
    sleep 1
  done
  if [ "$moved" != "1" ]; then
    record FAIL "VIP $VIP did not move within 30s"
  fi
  # restart keepalived on previous owner
  docker exec "$owner" keepalived -f /etc/keepalived/keepalived.conf -l 2>/dev/null || true
fi

# Static scripts
if bash -n "$ROOT/scripts/cluster/remote/apply-node.sh" \
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
  if [ "$skip" -gt 0 ]; then
    echo "UNEXPECTED SKIP in SSH release gate — treat as fail for release."
  fi
  echo "SSH Docker lab: live Apply + Patroni promote + unicast keepalived VIP."
} | tee -a "$OUT"

mkdir -p "D:/datasafe_tz/qa/2026-07-26" 2>/dev/null || true
cp "$OUT" "$QA_OUT" 2>/dev/null || true
cp "$OUT" "$STATE/drill-results-latest.md" 2>/dev/null || true
echo "  [OK] results: $OUT"

# Release gate: zero fail AND zero skip
[ "$fail" -eq 0 ] && [ "$skip" -eq 0 ]
