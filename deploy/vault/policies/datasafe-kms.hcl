# Data plane for one storage node. Rotate and delete are not included.
# The mount name is "transit". Change the prefix if TRANSIT_MOUNT is different.

path "transit/encrypt/datasafe-*" {
  capabilities = ["update"]
}

path "transit/decrypt/datasafe-*" {
  capabilities = ["update"]
}

path "transit/rewrap/datasafe-*" {
  capabilities = ["update"]
}

path "transit/keys/datasafe-*" {
  capabilities = ["create", "read", "update"]
}

# The connection test creates datasafe-connection-test when STORAGE_KMS_PROBE_KEY
# and STORAGE_KMS_DEFAULT_KEY are empty. It does not delete that key.
# Delete stays on the admin policy, which crypto-shred uses.
