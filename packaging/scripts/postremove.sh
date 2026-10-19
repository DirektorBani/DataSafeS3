#!/bin/sh
# Drop the unit from systemd's cache. Do not delete /var/lib/datasafes3.
# Object bytes and Bolt metadata in that directory stay after uninstall and purge.
set -eu

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
fi
