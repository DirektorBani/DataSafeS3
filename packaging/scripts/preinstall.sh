#!/bin/sh
# Create the system user before the package unpacks files owned by it.
# Runs on install and on upgrade. Safe to run again.
set -eu

if ! getent group datasafes3 >/dev/null 2>&1; then
  groupadd --system datasafes3
fi

shell=/usr/sbin/nologin
if [ ! -x "$shell" ]; then
  if [ -x /sbin/nologin ]; then
    shell=/sbin/nologin
  else
    shell=/bin/false
  fi
fi

if ! getent passwd datasafes3 >/dev/null 2>&1; then
  useradd --system \
    --gid datasafes3 \
    --home-dir /var/lib/datasafes3 \
    --no-create-home \
    --shell "$shell" \
    datasafes3
fi
