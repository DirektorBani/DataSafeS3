#!/usr/bin/env bash
# Dry-check that a tag push builds and publishes the storage-server image,
# including the Kafka bridge binary, with version and commit ldflags.
# This does not log in to GHCR and does not push.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
python3 "$ROOT/scripts/check_release_workflow.py"
