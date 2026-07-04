#!/bin/sh
# Periodic copy from primary storage volume to standby volumes (HA lab).
set -e

SOURCE=/source
DESTS="/dest1 /dest2"
INTERVAL="${SYNC_INTERVAL_SEC:-3}"

sync_all() {
  for dest in $DESTS; do
    mkdir -p "$dest"
    cp -a "${SOURCE}/." "${dest}/"
  done
}

sync_all
while true; do
  sleep "$INTERVAL"
  sync_all
done
