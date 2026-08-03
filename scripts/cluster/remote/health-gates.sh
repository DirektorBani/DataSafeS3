#!/usr/bin/env bash
# Post-apply health gates (best-effort). Run from operator host or any node.
# Usage: health-gates.sh --leader-ip IP [--vip-s3 IP] [--vip-pg IP]
set -euo pipefail

LEADER_IP=""
VIP_S3=""
VIP_PG=""
TIMEOUT=90

ok()   { echo "  [OK] $*"; }
warn() { echo "  [!!] $*"; }
err()  { echo "  [XX] $*"; return 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --leader-ip) LEADER_IP="$2"; shift 2 ;;
    --vip-s3) VIP_S3="$2"; shift 2 ;;
    --vip-pg) VIP_PG="$2"; shift 2 ;;
    --timeout) TIMEOUT="$2"; shift 2 ;;
    *) echo "unknown $1" >&2; exit 1 ;;
  esac
done

[ -n "$LEADER_IP" ] || { echo "--leader-ip required" >&2; exit 1; }

failed=0
check_http() {
  local url="$1" name="$2"
  if command -v curl >/dev/null 2>&1 && curl -fsS --max-time 5 "$url" >/dev/null 2>&1; then
    ok "$name $url"
  else
    warn "$name not ready: $url"
    failed=$((failed + 1))
  fi
}

check_http "http://${LEADER_IP}:9000/healthz" "storage leader"
check_http "http://${LEADER_IP}:8080/healthz" "console leader"
check_http "http://${LEADER_IP}:8008/patroni" "patroni rest" || \
  check_http "http://${LEADER_IP}:8008/primary" "patroni primary"

if [ -n "$VIP_S3" ]; then
  check_http "http://${VIP_S3}:9000/healthz" "VIP-S3"
fi
if [ -n "$VIP_PG" ]; then
  if command -v pg_isready >/dev/null 2>&1; then
    if pg_isready -h "$VIP_PG" -p 5432 >/dev/null 2>&1; then
      ok "VIP-Postgres $VIP_PG:5432"
    else
      warn "VIP-Postgres not ready"
      failed=$((failed + 1))
    fi
  else
    warn "pg_isready missing; skip VIP-Postgres TCP check"
  fi
fi

if [ "$failed" -gt 0 ]; then
  warn "health gates: $failed check(s) not green yet ( Patroni quorum may need more time )"
  exit 1
fi
ok "all health gates passed"
exit 0
