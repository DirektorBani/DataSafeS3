#!/usr/bin/env bash
# Fail when release pins do not match VERSION.
# Optional argument: pushed git tag (vX.Y.Z). VERSION must equal that tag.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec python3 "$ROOT/scripts/release_pins.py" check "$@"
