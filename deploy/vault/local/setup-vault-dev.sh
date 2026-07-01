#!/usr/bin/env bash
# Bootstrap HashiCorp Vault (dev mode) and seed KV-v2 secrets for DataSafeS3 local testing.
# Run from repository root. Requires Docker.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

COMPOSE=(docker compose -p datasafe
  -f docker-compose.yml
  -f docker-compose.vault.yml)

if [[ -f docker-compose.local-data.yml ]]; then
  COMPOSE+=(-f docker-compose.local-data.yml)
fi

echo "[vault-dev] Starting Vault (dev mode)..."
"${COMPOSE[@]}" --profile vault up -d vault

echo "[vault-dev] Seeding KV-v2 secrets (vault-init)..."
"${COMPOSE[@]}" --profile vault up vault-init

echo "[vault-dev] Done. Bring up the full stack:"
echo "  ${COMPOSE[*]} --profile vault up -d"
echo "Or run integration smoke: scripts/vault/smoke-vault-integration.sh"
echo "Vault UI/API: http://localhost:8200 (root token: root — dev only)"
