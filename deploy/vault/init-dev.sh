#!/bin/sh
# Lab only. Enables Transit and writes the two policies. The root token is not a production credential.
set -eu
export VAULT_ADDR="${VAULT_ADDR:-http://127.0.0.1:8200}"
export VAULT_TOKEN="${VAULT_TOKEN:-root}"

i=0
while [ "$i" -lt 40 ]; do
  if vault status >/dev/null 2>&1; then
    break
  fi
  i=$((i + 1))
  sleep 1
done
vault status >/dev/null

vault secrets enable transit >/dev/null 2>&1 || true
vault policy write datasafe-kms /policies/datasafe-kms.hcl
vault policy write datasafe-kms-admin /policies/datasafe-kms-admin.hcl

if [ -n "${TOKEN_OUT:-}" ]; then
  mkdir -p "$(dirname "$TOKEN_OUT")"
  printf '%s' "$VAULT_TOKEN" > "$TOKEN_OUT"
fi
