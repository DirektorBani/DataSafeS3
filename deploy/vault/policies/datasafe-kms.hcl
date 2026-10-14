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
