#!/usr/bin/env bash
# Fail when release pins do not match VERSION.
# Optional argument: pushed git tag (vX.Y.Z). VERSION must equal that tag.
# Uses python3 or python on the host. If neither exists, runs the checker
# in a python container (Git Bash does not rewrite the mount when MSYSTEM is set).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if command -v python3 >/dev/null 2>&1; then
  exec python3 "$ROOT/scripts/release_pins.py" check "$@"
fi
if command -v python >/dev/null 2>&1; then
  exec python "$ROOT/scripts/release_pins.py" check "$@"
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "release pin check needs python3, python, or docker" >&2
  exit 1
fi
if [ -n "${MSYSTEM:-}" ]; then
  export MSYS_NO_PATHCONV=1
  export MSYS2_ARG_CONV_EXCL='*'
fi
exec docker run --rm \
  -v "$ROOT:/work:ro" \
  -w /work \
  python:3.12-alpine \
  python /work/scripts/release_pins.py check "$@"
