#!/usr/bin/env bash
# Bring up Vault profile stack (optional) and run integration smoke.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

export VAULT_PROFILE=1
export TEST_VAULT_ADDR="${TEST_VAULT_ADDR:-http://127.0.0.1:8200}"
export STORAGE_URL="${STORAGE_URL:-http://127.0.0.1:9000}"

COMPOSE=(docker compose -p datasafe-vault
  -f docker-compose.yml
  -f docker-compose.vault.yml)

if [[ -f docker-compose.local-data.yml && -n "${DATASAFE_DATA_ROOT:-}" ]]; then
  COMPOSE+=(-f docker-compose.local-data.yml)
fi

PROFILES=(--profile vault)
if [[ "${VAULT_WITH_POSTGRES:-1}" == "1" ]]; then
  PROFILES+=(--profile postgres)
  export STORAGE_METADATA_BACKEND=postgres
fi

if [[ "${VAULT_INTEGRATION_UP:-1}" == "1" ]]; then
  echo "Starting Vault integration stack..."
  "${COMPOSE[@]}" "${PROFILES[@]}" up -d --wait vault vault-init vault-agent
  if [[ "${VAULT_WITH_POSTGRES:-1}" == "1" ]]; then
    "${COMPOSE[@]}" "${PROFILES[@]}" up -d --wait postgres
  fi
  "${COMPOSE[@]}" "${PROFILES[@]}" up -d --wait storage-server
fi

node scripts/vault/test-vault-integration.mjs
