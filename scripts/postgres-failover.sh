#!/usr/bin/env bash
# Promote PostgreSQL standby and repoint DataSafeS3 DSN (Community Edition HA drill helper)
set -euo pipefail

STANDBY_CONTAINER="${STANDBY_CONTAINER:-datasafe-postgres-standby}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-cursor_p}"
DRY_RUN="${DRY_RUN:-0}"

log() { echo "[postgres-failover] $*"; }

log "Stopping writes on primary storage-server"
if [[ "$DRY_RUN" != "1" ]]; then
  docker compose -p "$COMPOSE_PROJECT" stop storage-server 2>/dev/null || true
fi

log "Promoting standby PostgreSQL ($STANDBY_CONTAINER)"
if [[ "$DRY_RUN" == "1" ]]; then
  log "DRY RUN: would run SELECT pg_promote()"
else
  docker exec "$STANDBY_CONTAINER" psql -U datasafe -d datasafe -c "SELECT pg_promote(wait := false);"
fi

log "Waiting for promoted node"
for i in $(seq 1 30); do
  if docker exec "$STANDBY_CONTAINER" pg_isready -U datasafe -d datasafe >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

log "Restart storage-server with STORAGE_POSTGRES_DSN pointing at new primary"
log "Clear STORAGE_POSTGRES_READ_REPLICA_DSN until replica is rebuilt"

if [[ "$DRY_RUN" != "1" ]]; then
  docker compose -p "$COMPOSE_PROJECT" up -d storage-server
  for i in $(seq 1 60); do
    if curl -sf http://127.0.0.1:8080/healthz | grep -q '"status":"ok"'; then
      log "PASS: /healthz ok"
      exit 0
    fi
    sleep 2
  done
  log "FAIL: health check timeout"
  exit 1
fi

log "DRY RUN complete"
