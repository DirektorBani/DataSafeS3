#!/usr/bin/env bash
# Scripted Postgres promote for the compose HA lab.
# This is not automatic multi-AZ failover. A second run is safe.
#
# Exit codes (internal/ha/promote, scripts/cluster/promote.sh, EN/RU runbook):
#   0  done, already in that state, or dry-run / plan
#   1  usage
#   2  precheck (lag, unreachable standby, unnamed split brain)
#   3  fence failed (old primary still accepts a write)
#   4  promote failed
#   5  postcheck failed
#
# Fence: ALTER SYSTEM SET default_transaction_read_only=on on the named old
# primary only. postgresql.auto.conf is not replicated, so the promoted
# standby stays writable. Client backends are terminated. A new session cannot
# CREATE TABLE. storage-server then fails its leader renew and drops the lease
# immediately, so HTTP writes are rejected without waiting out the old TTL.
#
# Container names are the roles at the start of the incident. Pass the same
# names on a re-run. PRIMARY_CONTAINER is fenced. STANDBY_CONTAINER is promoted.
set -euo pipefail

PRIMARY_CONTAINER="${PRIMARY_CONTAINER:-}"
STANDBY_CONTAINER="${STANDBY_CONTAINER:-}"
PGUSER="${PGUSER:-datasafe}"
PGDATABASE="${PGDATABASE:-datasafe}"
MAX_LAG_BYTES="${MAX_LAG_BYTES:-1048576}"
PROMOTE_TIMING_FILE="${PROMOTE_TIMING_FILE:-}"
MODE="plan"

usage() {
  cat <<'EOF'
usage: promote.sh [--dry-run | --execute] [--primary NAME] [--standby NAME]
                  [--max-lag-bytes N] [--timing-file PATH]

  (no flag)     print the plan and exit 0 (no changes)
  --dry-run     print the plan and exit 0 (no changes)
  --execute     fence the old primary, promote the standby, clear ha_leader_lock

Exit codes: 0 ok or already done, 1 usage, 2 precheck, 3 fence, 4 promote, 5 postcheck
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) MODE="plan"; shift ;;
    --execute) MODE="execute"; shift ;;
    --primary) PRIMARY_CONTAINER="${2:-}"; shift 2 ;;
    --standby) STANDBY_CONTAINER="${2:-}"; shift 2 ;;
    --max-lag-bytes) MAX_LAG_BYTES="${2:-}"; shift 2 ;;
    --timing-file) PROMOTE_TIMING_FILE="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "promote: unknown argument: $1" >&2; usage >&2; exit 1 ;;
  esac
done

if [[ -n "${DRY_RUN:-}" && "${DRY_RUN}" != "0" ]]; then
  MODE="plan"
fi

log() { echo "[promote] $*"; }

psql_at() {
  local c="$1" sql="$2"
  docker exec "$c" psql -U "$PGUSER" -d "$PGDATABASE" -v ON_ERROR_STOP=1 -Atqc "$sql"
}

container_running() {
  local c="$1" state
  state="$(docker inspect -f '{{.State.Running}}' "$c" 2>/dev/null || true)"
  [[ "$state" == "true" ]]
}

in_recovery() {
  local c="$1" v
  v="$(psql_at "$c" "SELECT pg_is_in_recovery();" 2>/dev/null || true)"
  [[ "$v" == "t" ]]
}

read_only_session() {
  local c="$1" v
  v="$(psql_at "$c" "SELECT current_setting('transaction_read_only');" 2>/dev/null || true)"
  [[ "$v" == "on" ]]
}

# fenced when the process is down, still in recovery, or a new session cannot write.
is_fenced() {
  local c="$1"
  if ! container_running "$c"; then
    return 0
  fi
  if in_recovery "$c"; then
    return 0
  fi
  if read_only_session "$c"; then
    if psql_at "$c" "CREATE TABLE dr_fence_probe(id int);" >/dev/null 2>&1; then
      psql_at "$c" "DROP TABLE IF EXISTS dr_fence_probe;" >/dev/null 2>&1 || true
      return 1
    fi
    return 0
  fi
  return 1
}

lag_bytes() {
  local c="$1"
  psql_at "$c" "SELECT COALESCE((SELECT MAX(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn))::bigint FROM pg_stat_replication), -1);" 2>/dev/null || echo "-1"
}

apply_fence() {
  local c="$1"
  if ! container_running "$c"; then
    log "old primary $c is not running; it cannot accept writes"
    return 0
  fi
  if is_fenced "$c"; then
    log "old primary $c is already fenced"
    return 0
  fi
  log "fencing $c: default_transaction_read_only=on (local, not replicated)"
  if ! psql_at "$c" "ALTER SYSTEM SET default_transaction_read_only = on;" >/dev/null; then
    log "fence failed: ALTER SYSTEM on $c"
    return 3
  fi
  if ! psql_at "$c" "SELECT pg_reload_conf();" >/dev/null; then
    log "fence failed: reload on $c"
    return 3
  fi
  psql_at "$c" "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid() AND backend_type = 'client backend';" >/dev/null || true
  if is_fenced "$c"; then
    return 0
  fi
  log "fence failed: $c still accepts writes"
  return 3
}

promote_standby() {
  local c="$1"
  if ! container_running "$c"; then
    log "standby container $c is not running"
    return 2
  fi
  if ! in_recovery "$c"; then
    log "standby $c is already a primary"
    return 0
  fi
  log "promoting $c"
  if ! psql_at "$c" "SELECT pg_promote(wait := true);" >/dev/null; then
    log "pg_promote failed on $c"
    return 4
  fi
  local i
  for i in $(seq 1 30); do
    if ! in_recovery "$c"; then
      return 0
    fi
    sleep 1
  done
  log "standby $c still in recovery after promote"
  return 4
}

# A fresh promote deletes the previous lease so the new storage node can
# acquire it. A re-run must not delete a live lease: the new leader may
# already hold it, and dropping the row makes IsLeader false immediately.
clear_lock() {
  local c="$1" fresh="$2"
  if [[ "$fresh" != "yes" ]]; then
    log "re-run leaves a live ha_leader_lock on $c in place"
    return 0
  fi
  log "clearing previous ha_leader_lock on $c"
  psql_at "$c" "DO \$\$ BEGIN IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'ha_leader_lock') THEN DELETE FROM ha_leader_lock WHERE lock_id = 'storage-server'; END IF; END \$\$;" >/dev/null
}

can_write() {
  local c="$1"
  if psql_at "$c" "CREATE TABLE dr_fence_probe(id int);" >/dev/null 2>&1; then
    psql_at "$c" "DROP TABLE IF EXISTS dr_fence_probe;" >/dev/null 2>&1 || true
    return 0
  fi
  return 1
}

write_timing() {
  local started="$1" lag_before="$2" lag_after="$3" idempotent="$4"
  [[ -n "$PROMOTE_TIMING_FILE" ]] || return 0
  local finished now_s
  finished="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  now_s="$(date +%s)"
  mkdir -p "$(dirname "$PROMOTE_TIMING_FILE")"
  cat >"$PROMOTE_TIMING_FILE" <<EOF
{"started_at":"$started","finished_at":"$finished","seconds":$((now_s - started_epoch)),"lag_bytes_before":$lag_before,"lag_bytes_after":$lag_after,"idempotent_rerun":$idempotent,"mode":"$MODE"}
EOF
}

started_epoch="$(date +%s)"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

log "mode=$MODE primary=${PRIMARY_CONTAINER:-<unset>} standby=${STANDBY_CONTAINER:-<unset>} max_lag_bytes=$MAX_LAG_BYTES"
log "plan:"
log "  1. measure replication lag; refuse when the old primary is up and lag is above the limit"
log "  2. fence PRIMARY_CONTAINER with default_transaction_read_only=on (ALTER SYSTEM, not replicated)"
log "  3. pg_promote STANDBY_CONTAINER when it is still in recovery"
log "  4. on a fresh promote, DELETE ha_leader_lock on the new primary; a re-run leaves a live lease"
log "  5. postcheck: old primary refuses CREATE TABLE; new primary is writable and not in recovery"
log "lease: a failed leader renew marks that storage-server process not leader immediately"

if [[ "$MODE" != "execute" ]]; then
  if [[ -n "$PRIMARY_CONTAINER" ]] && container_running "$PRIMARY_CONTAINER"; then
    log "observed primary recovery=$(in_recovery "$PRIMARY_CONTAINER" && echo yes || echo no) fenced=$(is_fenced "$PRIMARY_CONTAINER" && echo yes || echo no) lag_bytes=$(lag_bytes "$PRIMARY_CONTAINER")"
  else
    log "observed primary: not queried (container unset or stopped)"
  fi
  if [[ -n "$STANDBY_CONTAINER" ]] && container_running "$STANDBY_CONTAINER"; then
    log "observed standby recovery=$(in_recovery "$STANDBY_CONTAINER" && echo yes || echo no)"
  else
    log "observed standby: not queried (container unset or stopped)"
  fi
  log "dry-run: no changes"
  write_timing "$started_at" "null" "null" "false"
  exit 0
fi

if [[ -z "$PRIMARY_CONTAINER" || -z "$STANDBY_CONTAINER" ]]; then
  echo "promote: --execute requires --primary and --standby" >&2
  exit 1
fi
if [[ "$PRIMARY_CONTAINER" == "$STANDBY_CONTAINER" ]]; then
  echo "promote: primary and standby are the same container" >&2
  exit 1
fi

if ! container_running "$STANDBY_CONTAINER"; then
  log "precheck: standby $STANDBY_CONTAINER is not running"
  exit 2
fi

idempotent="false"
lag_before="-1"
if container_running "$PRIMARY_CONTAINER" && ! in_recovery "$PRIMARY_CONTAINER"; then
  lag_before="$(lag_bytes "$PRIMARY_CONTAINER")"
fi

standby_recovery="no"
if in_recovery "$STANDBY_CONTAINER"; then
  standby_recovery="yes"
fi

if [[ "$standby_recovery" == "yes" ]]; then
  if ! container_running "$PRIMARY_CONTAINER"; then
    log "old primary is down; lag is unknown and promote continues"
  else
    if [[ "$lag_before" -lt 0 ]]; then
      log "precheck: no streaming replica visible from $PRIMARY_CONTAINER"
      exit 2
    fi
    if [[ "$lag_before" -gt "$MAX_LAG_BYTES" ]]; then
      log "precheck: lag ${lag_before} bytes is above ${MAX_LAG_BYTES}"
      exit 2
    fi
    log "lag before promote: ${lag_before} bytes"
  fi
else
  log "standby is already a primary; promote will be skipped"
  idempotent="true"
fi

set +e
apply_fence "$PRIMARY_CONTAINER"
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
  exit "$rc"
fi

set +e
promote_standby "$STANDBY_CONTAINER"
rc=$?
set -e
if [[ "$rc" -ne 0 ]]; then
  exit "$rc"
fi

fresh_lock="no"
if [[ "$idempotent" != "true" ]]; then
  fresh_lock="yes"
fi
if ! clear_lock "$STANDBY_CONTAINER" "$fresh_lock"; then
  log "postcheck: could not clear ha_leader_lock"
  exit 5
fi

if in_recovery "$STANDBY_CONTAINER"; then
  log "postcheck: $STANDBY_CONTAINER still in recovery"
  exit 5
fi
if ! can_write "$STANDBY_CONTAINER"; then
  log "postcheck: new primary $STANDBY_CONTAINER refused a write"
  exit 5
fi
if ! is_fenced "$PRIMARY_CONTAINER"; then
  log "postcheck: old primary $PRIMARY_CONTAINER is still writable"
  exit 3
fi

lag_after="-1"
if container_running "$STANDBY_CONTAINER"; then
  lag_after="$(lag_bytes "$STANDBY_CONTAINER")"
fi
log "postcheck ok; lag_bytes_after=$lag_after (no replica reads as -1)"
write_timing "$started_at" "$lag_before" "$lag_after" "$idempotent"
exit 0
