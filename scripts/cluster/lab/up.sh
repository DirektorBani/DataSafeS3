#!/usr/bin/env bash
# Bring up lab. Tries SSH-node build first; falls back to offline local images.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"

if docker build -t datasafe-cluster-lab-node:test \
  -f "$ROOT/deploy/cluster/lab/Dockerfile" "$ROOT/deploy/cluster/lab" \
  >/tmp/datasafe-lab-build.log 2>&1; then
  echo "  [OK] SSH node image build succeeded — using full SSH lab"
  bash "$ROOT/scripts/cluster/lab/up-ssh.sh"
else
  echo "  [!!] SSH node image build unavailable (registry/apk). Using offline local-image lab."
  echo "      (see /tmp/datasafe-lab-build.log)"
  bash "$ROOT/scripts/cluster/lab/up-local.sh"
fi
