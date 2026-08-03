#!/usr/bin/env bash
# Bootstrap datasafes3 user + SSH key (mode P). Run once as root via SSH.
# SECURITY: no password echo; ssh key mode 600; scoped sudoers only.
set -euo pipefail

USER_NAME="${DATASAFE_USER:-datasafes3}"
HOME_DIR="/home/${USER_NAME}"
SSH_DIR="${HOME_DIR}/.ssh"
PUBKEY_B64="${DATASAFE_SSH_PUBKEY_B64:-}"

if [ "$(id -u)" -ne 0 ]; then
  echo "must run as root" >&2
  exit 1
fi

if ! id -u "$USER_NAME" >/dev/null 2>&1; then
  useradd --create-home --shell /bin/bash "$USER_NAME"
fi

install -d -m 700 -o "$USER_NAME" -g "$USER_NAME" "$SSH_DIR"
if [ -n "$PUBKEY_B64" ]; then
  echo "$PUBKEY_B64" | base64 -d >"${SSH_DIR}/authorized_keys"
  chown "$USER_NAME:$USER_NAME" "${SSH_DIR}/authorized_keys"
  chmod 600 "${SSH_DIR}/authorized_keys"
fi

# Package hints (distro-specific Apply installs real packages)
mkdir -p /var/lib/datasafe/{etcd,postgresql,erasure,nfs-export}
chown -R "$USER_NAME:$USER_NAME" /var/lib/datasafe

echo "bootstrap ok: user=$USER_NAME"
