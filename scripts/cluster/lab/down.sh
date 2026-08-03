#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
STATE="${HOME:-/tmp}/.datasafe-cluster"
COMPOSE="$STATE/docker-compose.lab-local.yml"
[ -f "$COMPOSE" ] || COMPOSE="$ROOT/deploy/cluster/lab/docker-compose.local.yml"
docker compose -p datasafe-cluster-lab-local -f "$COMPOSE" down -v 2>/dev/null || true
docker compose -p datasafe-cluster-lab -f "$ROOT/deploy/cluster/lab/docker-compose.yml" down -v 2>/dev/null || true
echo "  [OK] lab down"
