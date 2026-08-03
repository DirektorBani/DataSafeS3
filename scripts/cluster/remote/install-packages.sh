#!/usr/bin/env bash
# Install cluster packages (idempotent-ish). Run as root on each node.
# SECURITY: no secrets; apt/dnf only.
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "must run as root" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

install_debian() {
  apt-get update -y
  apt-get install -y --no-install-recommends \
    curl ca-certificates nfs-kernel-server nfs-common \
    haproxy keepalived \
    postgresql postgresql-contrib \
    etcd \
    python3 python3-pip python3-psycopg2 \
    sudo acl
  # Patroni via pip if package missing
  if ! command -v patroni >/dev/null 2>&1; then
    pip3 install --break-system-packages 'patroni[etcd3]' 2>/dev/null \
      || pip3 install 'patroni[etcd3]'
  fi
}

install_rhel() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y curl ca-certificates nfs-utils haproxy keepalived \
      postgresql-server postgresql-contrib etcd python3 python3-pip sudo
  else
    yum install -y curl ca-certificates nfs-utils haproxy keepalived \
      postgresql-server postgresql-contrib etcd python3 python3-pip sudo
  fi
  if ! command -v patroni >/dev/null 2>&1; then
    pip3 install 'patroni[etcd3]'
  fi
}

if [ -f /etc/debian_version ]; then
  install_debian
elif [ -f /etc/redhat-release ]; then
  install_rhel
else
  echo "unsupported distro; install etcd patroni postgres haproxy keepalived nfs manually" >&2
  exit 1
fi

mkdir -p /var/lib/datasafe/{etcd,postgresql,erasure,nfs-export,apply}
mkdir -p /etc/datasafe
echo "packages ok"
