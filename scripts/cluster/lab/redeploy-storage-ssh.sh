#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
KEYS="$ROOT/deploy/cluster/lab/keys/id_ed25519"
KH="${HOME:-/tmp}/.datasafe-cluster/lab_known_hosts"
SSH=(ssh -n -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="$KH" -i "$KEYS" -p 2221 datasafes3@127.0.0.1)
SCP=(scp -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="$KH" -i "$KEYS" -P 2221)

docker rm -f datasafe-storage-leader >/dev/null 2>&1 || true

# Write a remote fixer script to avoid quoting hell
cat >/tmp/fix-pg-secret.sh <<'EOS'
#!/bin/sh
set -eu
PG_SUPER=$(awk '/superuser:/{f=1} f && /password:/{gsub(/.*password:[[:space:]]*/,""); gsub(/["\r]/,""); print; exit}' /etc/datasafe/patroni.yml)
echo "PG_SUPER_PASSWORD=${PG_SUPER}" >/etc/datasafe/cluster-secrets.env
chmod 600 /etc/datasafe/cluster-secrets.env
cat /etc/datasafe/cluster-secrets.env
EOS

"${SCP[@]}" /tmp/fix-pg-secret.sh datasafes3@127.0.0.1:/tmp/fix-pg-secret.sh
"${SSH[@]}" 'sudo sh /tmp/fix-pg-secret.sh'
"${SCP[@]}" "$ROOT/scripts/cluster/remote/deploy-storage-server.sh" \
  datasafes3@127.0.0.1:/var/tmp/datasafe-apply/remote/deploy-storage-server.sh
"${SSH[@]}" 'sudo bash /var/tmp/datasafe-apply/remote/deploy-storage-server.sh --leader-ip 10.88.0.10 --bundle /var/tmp/datasafe-apply/bundle'
curl -fsS --max-time 5 http://127.0.0.1:9010/healthz
echo
echo "storage ok"
