#!/bin/sh
# Lab only. Transit plus one AppRole per storage node and one admin AppRole.
# Role ids and secret ids are fixed so compose can pass them without reading
# Vault. They are not production credentials.
set -eu
export VAULT_ADDR="${VAULT_ADDR:-http://vault:8200}"
export VAULT_TOKEN="${VAULT_TOKEN:-root}"

i=0
while [ "$i" -lt 60 ]; do
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
vault auth enable approle >/dev/null 2>&1 || true

write_role() {
  name="$1"
  policy="$2"
  role_id="$3"
  secret_id="$4"
  vault write "auth/approle/role/${name}" \
    token_policies="$policy" \
    token_ttl=4h \
    token_max_ttl=24h \
    secret_id_ttl=0 >/dev/null
  vault write "auth/approle/role/${name}/role-id" role_id="$role_id" >/dev/null
  vault write "auth/approle/role/${name}/custom-secret-id" secret_id="$secret_id" >/dev/null
}

write_role node1 datasafe-kms node1-role-id node1-secret-id
write_role node2 datasafe-kms node2-role-id node2-secret-id
write_role node3 datasafe-kms node3-role-id node3-secret-id
write_role kmsadmin datasafe-kms-admin kms-admin-role kms-admin-secret
echo ready
