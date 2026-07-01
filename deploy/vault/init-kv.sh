#!/bin/sh
# One-shot: seed KV v2 paths for Vault local dev / CI (fixtures only — not production).
set -eu

VAULT_ADDR="${VAULT_ADDR:-http://vault:8200}"
VAULT_TOKEN="${VAULT_TOKEN:-root}"

# shellcheck disable=SC1091
. /vault/config/test-fixtures.env

echo "vault-init: waiting for Vault at ${VAULT_ADDR}..."
for _ in $(seq 1 60); do
  if vault status >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! vault status >/dev/null 2>&1; then
  echo "vault-init: Vault not reachable" >&2
  exit 1
fi

vault secrets enable -path=secret kv-v2 2>/dev/null || true

PG_DSN="postgres://datasafe:${VAULT_TEST_PG_PASSWORD}@postgres:5432/datasafe?sslmode=disable"

echo "vault-init: writing bootstrap bundle ${VAULT_KV_PATH}"
vault kv put "${VAULT_KV_PATH}" \
  jwt_secret="${VAULT_TEST_JWT_SECRET}" \
  s3_secret_key="${VAULT_TEST_S3_SECRET}" \
  admin_password="${VAULT_TEST_ADMIN_PASSWORD}" \
  mfa_encryption_key="${VAULT_TEST_MFA_KEY}" \
  sse_master_key="${VAULT_TEST_SSE_MASTER}" \
  postgres_password="${VAULT_TEST_PG_PASSWORD}" \
  postgres_dsn="${PG_DSN}" \
  oidc_client_secret="${VAULT_TEST_OIDC_SECRET}" \
  ldap_bind_password="${VAULT_TEST_LDAP_BIND}"

echo "vault-init: writing per-secret KV paths (secret/datasafe/*)"
vault kv put secret/datasafe/jwt value="${VAULT_TEST_JWT_SECRET}"
vault kv put secret/datasafe/s3-secret value="${VAULT_TEST_S3_SECRET}"
vault kv put secret/datasafe/admin-password value="${VAULT_TEST_ADMIN_PASSWORD}"
vault kv put secret/datasafe/mfa-encryption value="${VAULT_TEST_MFA_KEY}"
vault kv put secret/datasafe/sse-master value="${VAULT_TEST_SSE_MASTER}"
vault kv put secret/datasafe/postgres password="${VAULT_TEST_PG_PASSWORD}"
vault kv put secret/datasafe/postgres-dsn dsn="${PG_DSN}"
vault kv put secret/datasafe/oidc-client-secret value="${VAULT_TEST_OIDC_SECRET}"
vault kv put secret/datasafe/ldap-bind value="${VAULT_TEST_LDAP_BIND}"

echo "vault-init: done"
