#!/bin/sh
# Stop the service only when the package is being removed.
# Debian prerm: $1 is "remove" or "upgrade".
# RPM %preun: $1 is 0 on uninstall and 1 on upgrade.
set -eu

removing=0
case "${1:-}" in
  remove|0) removing=1 ;;
esac

if [ "$removing" -eq 1 ] && command -v systemctl >/dev/null 2>&1; then
  systemctl stop datasafes3.service >/dev/null 2>&1 || true
  systemctl disable datasafes3.service >/dev/null 2>&1 || true
fi
