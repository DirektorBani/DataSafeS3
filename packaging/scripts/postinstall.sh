#!/bin/sh
# Reload systemd so the new unit is visible. Do not enable or start it.
# A service the operator already started is restarted on upgrade.
set -eu

if [ -d /var/lib/datasafes3 ]; then
  chown datasafes3:datasafes3 /var/lib/datasafes3
  chmod 0750 /var/lib/datasafes3
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload >/dev/null 2>&1 || true
  if systemctl is-active --quiet datasafes3.service 2>/dev/null; then
    systemctl try-restart datasafes3.service >/dev/null 2>&1 || true
  fi
fi
