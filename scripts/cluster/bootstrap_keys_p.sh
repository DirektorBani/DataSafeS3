#!/usr/bin/env bash
# Mode [P] key bootstrap: create local ed25519, install datasafes3 + pubkey on each node via root once.
# SECURITY: password only via SSH_ASKPASS (never argv / never logged). Compatible with bash 3.2+.
set -euo pipefail

INVENTORY=""
IDENTITY=""
PUBKEY=""
ROOT_USER="root"
NODES_FILE=""

ok()   { printf '  [OK] %s\n' "$*" >&2; }
warn() { printf '  [!!] %s\n' "$*" >&2; }
err()  { printf '  [XX] %s\n' "$*" >&2; exit 1; }
info() { printf '  %s\n' "$*" >&2; }

usage() {
  cat >&2 <<'EOF'
Usage: bootstrap_keys_p.sh --inventory <path> [--identity ~/.ssh/datasafe_ed25519] [--nodes-file map.txt]

Creates (if missing) an ed25519 key, prompts once for root password, then on each node:
  - runs create-datasafes3.sh with DATASAFE_SSH_PUBKEY_B64
  - verifies BatchMode SSH as datasafes3

After success, run Apply with: --identity <same key>
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --inventory) INVENTORY="$2"; shift 2 ;;
    --identity) IDENTITY="$2"; shift 2 ;;
    --pubkey) PUBKEY="$2"; shift 2 ;;
    --root-user) ROOT_USER="$2"; shift 2 ;;
    --nodes-file) NODES_FILE="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) err "unknown: $1" ;;
  esac
done

[ -n "$INVENTORY" ] && [ -f "$INVENTORY" ] || err "--inventory required"
command -v ssh >/dev/null || err "ssh required"
command -v scp >/dev/null || err "scp required"
command -v ssh-keygen >/dev/null || err "ssh-keygen required"

if [ -z "$IDENTITY" ]; then
  IDENTITY="${HOME}/.ssh/datasafe_ed25519"
fi
if [ -z "$PUBKEY" ]; then
  PUBKEY="${IDENTITY}.pub"
fi

mkdir -p "$(dirname "$IDENTITY")"
if [ ! -f "$IDENTITY" ]; then
  ssh-keygen -t ed25519 -N "" -f "$IDENTITY" -C "datasafes3-cluster" >/dev/null
  ok "generated $IDENTITY"
else
  ok "using existing $IDENTITY"
fi
[ -f "$PUBKEY" ] || err "missing pubkey: $PUBKEY"

PUB_B64="$(base64 <"$PUBKEY" | tr -d '\n\r')"
[ -n "$PUB_B64" ] || err "empty pubkey"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BOOT_SRC="$SCRIPT_DIR/../../deploy/cluster/templates/bootstrap/create-datasafes3.sh"
[ -f "$BOOT_SRC" ] || err "missing $BOOT_SRC"

NODES_TMP="$(mktemp)"
ASKPASS="$(mktemp)"
PASSFILE="$(mktemp)"
trap 'rm -f "$NODES_TMP" "$ASKPASS" "$PASSFILE"' EXIT
chmod 700 "$ASKPASS" "$PASSFILE"

if [ -n "$NODES_FILE" ] && [ -f "$NODES_FILE" ]; then
  grep -v '^\s*#' "$NODES_FILE" | grep -v '^\s*$' >"$NODES_TMP" || true
else
  # fabric_ip from inventory nodes[].ip (simple sed; Wave 1 schema)
  sed -n 's/.*"ip"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$INVENTORY" | while read -r ip; do
    [ -n "$ip" ] || continue
    echo "$ip $ip 22"
  done >"$NODES_TMP"
fi
[ -s "$NODES_TMP" ] || err "no nodes in inventory"

info "Enter root password once (not echoed; not saved to inventory):"
# portable hidden read
if [ -t 0 ]; then
  stty -echo
  IFS= read -r ROOT_PASS || true
  stty echo
  printf '\n' >&2
else
  err "need interactive TTY for password"
fi
[ -n "${ROOT_PASS:-}" ] || err "empty password"
printf '%s\n' "$ROOT_PASS" >"$PASSFILE"
unset ROOT_PASS

cat >"$ASKPASS" <<EOF
#!/bin/sh
cat "$PASSFILE"
EOF
chmod 700 "$ASKPASS"

export SSH_ASKPASS="$ASKPASS"
export SSH_ASKPASS_REQUIRE=force
export DISPLAY="${DISPLAY:-datasafe-askpass}"

fail=0
while read -r fabric_ip ssh_host ssh_port; do
  [ -n "$fabric_ip" ] || continue
  ssh_port="${ssh_port:-22}"
  ssh_host="${ssh_host:-$fabric_ip}"
  info "bootstrap $fabric_ip via ${ROOT_USER}@${ssh_host}:${ssh_port}"
  if ! SSH_ASKPASS="$ASKPASS" SSH_ASKPASS_REQUIRE=force \
    scp -o StrictHostKeyChecking=accept-new -o NumberOfPasswordPrompts=1 -P "$ssh_port" \
      "$BOOT_SRC" "${ROOT_USER}@${ssh_host}:/tmp/create-datasafes3.sh"; then
    warn "scp failed: $ssh_host"; fail=$((fail + 1)); continue
  fi
  if ! SSH_ASKPASS="$ASKPASS" SSH_ASKPASS_REQUIRE=force \
    ssh -o StrictHostKeyChecking=accept-new -o NumberOfPasswordPrompts=1 -p "$ssh_port" \
      "${ROOT_USER}@${ssh_host}" \
      "DATASAFE_SSH_PUBKEY_B64='${PUB_B64}' bash /tmp/create-datasafes3.sh && rm -f /tmp/create-datasafes3.sh"; then
    warn "bootstrap failed: $ssh_host"; fail=$((fail + 1)); continue
  fi
  if ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i "$IDENTITY" -p "$ssh_port" \
      "datasafes3@${ssh_host}" "echo ok" >/dev/null 2>&1; then
    ok "datasafes3 BatchMode OK on $ssh_host"
  else
    warn "BatchMode verify failed on $ssh_host (sudo may still be needed for Apply)"
    fail=$((fail + 1))
  fi
done <"$NODES_TMP"

# wipe password file ASAP
: >"$PASSFILE"
rm -f "$PASSFILE"

if [ "$fail" -gt 0 ]; then
  err "$fail node(s) failed key bootstrap"
fi
ok "Mode P key bootstrap complete. Use: --identity $IDENTITY"
