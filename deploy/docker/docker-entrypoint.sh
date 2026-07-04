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
  if [ "${STORAGE_READ_ONLY:-false}" != "true" ]; then
    mkdir -p "$DATA_DIR/objects/buckets"
    chown -R "${NONROOT_UID}:${NONROOT_GID}" "$DATA_DIR"
  fi
  # Erasure shard volumes are separate mounts; ensure nonroot can write shards.
  if [ "${STORAGE_OBJECT_BACKEND:-fs}" = "erasure" ]; then
    if [ -n "${STORAGE_ERASURE_DATA_PATHS:-}" ]; then
      OLDIFS=$IFS
      IFS=,
      for shard_path in $STORAGE_ERASURE_DATA_PATHS; do
        shard_path=$(echo "$shard_path" | tr -d ' ')
        if [ -n "$shard_path" ]; then
          mkdir -p "$shard_path"
          chown -R "${NONROOT_UID}:${NONROOT_GID}" "$shard_path"
        fi
      done
      IFS=$OLDIFS
    elif [ -d /shards ]; then
      chown -R "${NONROOT_UID}:${NONROOT_GID}" /shards
    fi
  fi
  exec su-exec "${NONROOT_UID}:${NONROOT_GID}" "$SERVER_BIN" "$@"
fi

exec "$SERVER_BIN" "$@"