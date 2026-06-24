#!/bin/sh
set -e

DATA_DIR="${STORAGE_DATA_DIR:-/data}"
NONROOT_UID=65532
NONROOT_GID=65532

# Optional bind-mounted dev binary (Windows hosts cannot exec-mount over /storage-server directly).
SERVER_BIN=/storage-server
if [ -f /storage-server-bin ]; then
  cp /storage-server-bin /tmp/storage-server
  chmod 755 /tmp/storage-server
  SERVER_BIN=/tmp/storage-server
fi

if [ "$(id -u)" = "0" ]; then
  mkdir -p "$DATA_DIR/objects/buckets"
  chown -R "${NONROOT_UID}:${NONROOT_GID}" "$DATA_DIR"
  exec su-exec "${NONROOT_UID}:${NONROOT_GID}" "$SERVER_BIN" "$@"
fi

exec "$SERVER_BIN" "$@"