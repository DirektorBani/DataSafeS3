#!/usr/bin/env bash
# Generate a local X25519 KEK keypair for field encryption dev/testing.
# NEVER commit private keys. Output directory data/keys/ is gitignored via data/.
set -euo pipe
fail() { echo "ERROR: $*" >&2; exit 1; }

command -v openssl >/dev/null 2>&1 || fail "openssl is required"

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
KEY_DIR="${DATASAFE_KEK_DIR:-$ROOT/data/keys}"
KEK_ID="${DATASAFE_KEK_ID:-kek-$(date +%Y%m%d)-$(openssl rand -hex 2)}"
KEY_FILE="$KEY_DIR/${KEK_ID}.key"
PUB_FILE="$KEY_DIR/${KEK_ID}.pub"

mkdir -p "$KEY_DIR"
chmod 700 "$KEY_DIR"

if [[ -f "$KEY_FILE" || -f "$PUB_FILE" ]]; then
  fail "Refusing to overwrite existing keys in $KEY_DIR (set DATASAFE_KEK_ID or remove files)"
fi

openssl genpkey -algorithm X25519 -out "$KEY_FILE"
chmod 600 "$KEY_FILE"
openssl pkey -in "$KEY_FILE" -pubout -out "$PUB_FILE"
chmod 644 "$PUB_FILE"

# PKCS#8 DER for X25519: last 32 bytes are the raw private seed / public key material.
PRIV_B64="$(openssl pkey -in "$KEY_FILE" -outform DER | tail -c 32 | base64 | tr -d '\n')"
PUB_B64="$(openssl pkey -pubin -in "$PUB_FILE" -outform DER | tail -c 32 | base64 | tr -d '\n')"

echo ""
echo "=== DataSafeS3 field encryption KEK (local only) ==="
echo "kek_id suggestion: ${KEK_ID}"
echo "Private PEM: $KEY_FILE"
echo "Public PEM:  $PUB_FILE"
echo ""
echo "Add to .env (NEVER commit):"
echo "STORAGE_FIELD_ENCRYPTION_ENABLED=true"
echo "STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID=${KEK_ID}"
echo "STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY=${PRIV_B64}"
echo ""
echo "Public key (base64 raw 32 bytes, for registry bootstrap):"
echo "STORAGE_FIELD_ENCRYPTION_KEK_PUBLIC_KEY=${PUB_B64}"
echo ""
echo "Rotation: keep old private keys in STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS JSON until rewrap complete."
