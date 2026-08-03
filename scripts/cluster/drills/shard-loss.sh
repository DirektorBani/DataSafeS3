#!/usr/bin/env bash
# Drill: wipe one shard path and verify object GET still works (operator supplies curl/aws cli).
set -euo pipefail
SHARD_PATH="${1:-/var/lib/datasafe/erasure/shard0}"
S3_ENDPOINT="${2:-http://127.0.0.1:9000}"
echo "Wiping $SHARD_PATH (keep dir)"
rm -rf "${SHARD_PATH:?}/"*
echo "Probe healthz: $S3_ENDPOINT/healthz"
curl -fsS "$S3_ENDPOINT/healthz"
echo ""
echo "Next: PUT/GET a test object via S3 client against $S3_ENDPOINT"
