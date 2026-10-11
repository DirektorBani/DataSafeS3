#!/usr/bin/env bash
# Scripted Patroni switchover for the cluster lab.
# This is not automatic multi-AZ failover. A second run is safe: if the
# requested candidate is already the primary, the script exits 0.
#
# Exit codes match scripts/ha/promote.sh and internal/ha/promote:
#   0  done, already leader, or dry-run / plan
#   1  usage
#   2  precheck (patronictl missing, candidate unknown)
#   3  fence failed (old primary still accepts writes / still primary)
#   4  switchover command failed
#   5  postcheck failed (candidate did not become primary)
#
# Fence: Patroni demotes the old primary. That node re-enters recovery and
# refuses writes. This script does not issue ALTER SYSTEM; the compose HA lab
# uses scripts/ha/promote.sh for that fence.
set -euo pipefail

MODE="plan"
CANDIDATE="${PATRONI_CANDIDATE:-}"
CONTAINER="${PATRONI_CONTAINER:-}"
SCOPE="${PATRONI_SCOPE:-datasafe}"
CONFIG="${PATRONI_CONFIG:-/etc/datasafe/patroni.yml}"

usage() {
  cat <<'EOF'
usage: promote.sh [--dry-run | --execute] [--candidate NAME] [--container NAME]

  (no flag)   print the plan and exit 0
  --dry-run   print the plan and exit 0
  --execute   patronictl switchover inside PATRONI_CONTAINER

Exit codes: 0 ok or already done, 1 usage, 2 precheck, 3 fence, 4 promote, 5 postcheck
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) MODE="plan"; shift ;;
    --execute) MODE="execute"; shift ;;
    --candidate) CANDIDATE="${2:-}"; shift 2 ;;
    --container) CONTAINER="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "cluster-promote: unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
done

log() { echo "[cluster-promote] $*"; }

log "mode=$MODE candidate=${CANDIDATE:-<current replica>} container=${CONTAINER:-<unset>}"
log "plan:"
log "  1. patronictl list — find the current primary and the candidate"
log "  2. if the candidate is already primary, exit 0"
log "  3. patronictl switchover --leader <current> --candidate <node> --force"
log "  4. Patroni demotes the old primary (it re-enters recovery and refuses writes)"
log "  5. postcheck: candidate answers /primary; the previous primary does not"

if [[ "$MODE" != "execute" ]]; then
  log "dry-run: no changes"
  exit 0
fi

if [[ -z "$CONTAINER" ]]; then
  log "precheck: set PATRONI_CONTAINER or --container"
  exit 2
fi
if ! docker inspect "$CONTAINER" >/dev/null 2>&1; then
  log "precheck: container $CONTAINER not found"
  exit 2
fi

list="$(docker exec "$CONTAINER" patronictl -c "$CONFIG" list "$SCOPE" 2>/dev/null || true)"
if [[ -z "$list" ]]; then
  log "precheck: patronictl list failed in $CONTAINER"
  exit 2
fi
log "$list"

if [[ -n "$CANDIDATE" ]] && printf '%s\n' "$list" | grep -E -q "^[[:space:]]*\\|?[[:space:]]*${CANDIDATE}[[:space:]]+\\|[^|]*Leader"; then
  log "$CANDIDATE is already Leader"
  exit 0
fi

leader="$(printf '%s\n' "$list" | awk -F'|' 'tolower($0) ~ /leader/ { gsub(/ /, "", $2); print $2; exit }')"
if [[ -z "$leader" ]]; then
  log "precheck: could not read the current leader"
  exit 2
fi
if [[ -z "$CANDIDATE" ]]; then
  log "precheck: --candidate is required for --execute"
  exit 2
fi
if [[ "$leader" == "$CANDIDATE" ]]; then
  log "$CANDIDATE is already the leader"
  exit 0
fi

log "switchover $leader -> $CANDIDATE"
if ! docker exec "$CONTAINER" patronictl -c "$CONFIG" switchover "$SCOPE" --leader "$leader" --candidate "$CANDIDATE" --force; then
  log "switchover command failed"
  exit 4
fi

ok="no"
for _ in $(seq 1 60); do
  if docker exec "$CONTAINER" patronictl -c "$CONFIG" list "$SCOPE" 2>/dev/null | grep -E -q "${CANDIDATE}[[:space:]]+\\|[^|]*Leader"; then
    ok="yes"
    break
  fi
  sleep 1
done
if [[ "$ok" != "yes" ]]; then
  log "postcheck: $CANDIDATE did not become leader"
  exit 5
fi

still="$(docker exec "$CONTAINER" patronictl -c "$CONFIG" list "$SCOPE" 2>/dev/null || true)"
if printf '%s\n' "$still" | awk -F'|' -v n="$leader" 'gsub(/ /, "", $2) && $2==n && tolower($0) ~ /leader/ { found=1 } END { exit found?0:1 }'; then
  log "fence failed: $leader is still leader"
  exit 3
fi
log "switchover complete; $leader is no longer leader"
exit 0
