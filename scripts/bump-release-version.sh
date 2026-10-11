#!/usr/bin/env bash
# Bump every release pin to X.Y.Z and rewrite VERSION.
# Usage: scripts/bump-release-version.sh 1.4.2
set -euo pipefail
if [ $# -ne 1 ]; then
  echo "usage: scripts/bump-release-version.sh X.Y.Z" >&2
  exit 2
fi
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec python3 "$ROOT/scripts/release_pins.py" bump "$1"
