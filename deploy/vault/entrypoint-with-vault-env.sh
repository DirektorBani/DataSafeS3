#!/bin/sh
# Sources Vault Agent rendered env before starting storage-server.
# When DATASAFE_VAULT_REQUIRED is not true and the file is absent, falls through to stock entrypoint.
set -eu

ENV_FILE="${VAULT_RENDERED_ENV:-/rendered/datasafe.env}"

if [ ! -s "$ENV_FILE" ]; then
  if [ "${DATASAFE_VAULT_REQUIRED:-}" = "true" ]; then
    for _ in $(seq 1 90); do
      if [ -s "$ENV_FILE" ]; then
        break
      fi
      sleep 1
    done
  fi
fi

if [ ! -s "$ENV_FILE" ]; then
  if [ "${DATASAFE_VAULT_REQUIRED:-}" = "true" ]; then
    echo "vault entrypoint: missing rendered env file: $ENV_FILE" >&2
    exit 1
  fi
  exec /docker-entrypoint.sh "$@"
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

exec /docker-entrypoint.sh "$@"
