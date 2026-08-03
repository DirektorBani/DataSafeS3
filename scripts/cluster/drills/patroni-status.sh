#!/usr/bin/env bash
# Drill: Patroni primary check / promote hint (works on real VMs with patronictl).
set -euo pipefail
LEADER_IP="${1:-}"
[ -n "$LEADER_IP" ] || { echo "usage: $0 <any-node-ip>" >&2; exit 1; }
if command -v patronictl >/dev/null 2>&1; then
  patronictl -c /etc/datasafe/patroni.yml list || true
  echo "To promote a replica: patronictl -c /etc/datasafe/patroni.yml failover --force"
else
  curl -fsS "http://${LEADER_IP}:8008/patroni" || curl -fsS "http://${LEADER_IP}:8008/primary"
fi
