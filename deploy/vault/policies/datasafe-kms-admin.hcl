# Operator role for rotate and delete. Crypto-shred jobs are a later step.
# deletion_allowed is a POST to the key config path.

path "transit/keys/datasafe-*" {
  capabilities = ["read", "update", "delete"]
}

path "transit/keys/datasafe-*/rotate" {
  capabilities = ["update"]
}

path "transit/keys/datasafe-*/config" {
  capabilities = ["read", "update"]
}
