#!/usr/bin/env bash
set -euo pipefail

mkdir -p /home/datasafes3/.ssh /root/.ssh /var/run/sshd
chmod 700 /home/datasafes3/.ssh /root/.ssh

if [ -n "${LAB_SSH_PUBKEY:-}" ]; then
  echo "$LAB_SSH_PUBKEY" > /home/datasafes3/.ssh/authorized_keys
  echo "$LAB_SSH_PUBKEY" > /root/.ssh/authorized_keys
elif [ -f /lab-keys/id_ed25519.pub ]; then
  cp /lab-keys/id_ed25519.pub /home/datasafes3/.ssh/authorized_keys
  cp /lab-keys/id_ed25519.pub /root/.ssh/authorized_keys
fi

if [ -f /home/datasafes3/.ssh/authorized_keys ]; then
  chown -R datasafes3:datasafes3 /home/datasafes3/.ssh
  chmod 600 /home/datasafes3/.ssh/authorized_keys /root/.ssh/authorized_keys 2>/dev/null || true
fi

# sshd config
cat >/etc/ssh/sshd_config <<'EOF'
Port 22
PermitRootLogin prohibit-password
PasswordAuthentication no
PubkeyAuthentication yes
Subsystem sftp /usr/lib/ssh/sftp-server
EOF

touch /etc/datasafe/lab-mode
mkdir -p /var/lib/datasafe/{etcd,postgresql,erasure,nfs-export,apply}

# Ensure postgres user exists for patroni ownership attempts
adduser -D -s /bin/sh postgres 2>/dev/null || true

# Alpine locks passwordless accounts (! in shadow) — pubkey auth needs an unlocked account.
# '*' disables password auth without locking (unlike '!').
usermod -p '*' datasafes3 2>/dev/null || sed -i 's|^datasafes3:!:|datasafes3:*:|' /etc/shadow
usermod -p '*' root 2>/dev/null || sed -i 's|^root:!:|root:*:|' /etc/shadow || true

exec /usr/sbin/sshd -D -e
