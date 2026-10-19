#!/usr/bin/env bash
# Install one datasafes3 package inside a clean distro container, exercise it,
# then uninstall and confirm /var/lib/datasafes3 was kept.
# Usage: package-install-check.sh /path/to/package deb|rpm X.Y.Z
set -euo pipefail

pkg="${1:?package path}"
kind="${2:?deb or rpm}"
version="${3:?version}"

if [ ! -f "$pkg" ]; then
  echo "package not found: $pkg" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

case "$kind" in
  deb)
    apt-get update
    apt-get install -y systemd curl ca-certificates
    apt-get install -y "$pkg"
    ;;
  rpm)
    # Rocky and Alma ship curl-minimal, which conflicts with the curl package.
    dnf install -y systemd shadow-utils
    if ! command -v curl >/dev/null 2>&1; then
      dnf install -y curl-minimal || dnf install -y curl
    fi
    dnf install -y "$pkg"
    ;;
  *)
    echo "unknown kind $kind" >&2
    exit 1
    ;;
esac

echo "== user"
getent passwd datasafes3
uid="$(id -u datasafes3)"
test "$uid" -lt 1000
test "$(id -gn datasafes3)" = "datasafes3"
shell="$(getent passwd datasafes3 | awk -F: '{print $7}')"
case "$shell" in
  */nologin|*/false) ;;
  *)
    echo "datasafes3 shell is $shell, want nologin or false" >&2
    exit 1
    ;;
esac

echo "== files"
test -x /usr/bin/datasafe-storage-server
test -x /usr/bin/storage-cli
test -x /usr/lib/datasafe/datasafe-kafka-bridge
test -f /usr/lib/systemd/system/datasafes3.service
test -d /var/lib/datasafes3
mode="$(stat -c '%a %U %G' /etc/datasafes3/datasafes3.env)"
test "$mode" = "640 root datasafes3"
dmode="$(stat -c '%a %U %G' /var/lib/datasafes3)"
test "$dmode" = "750 datasafes3 datasafes3"

echo "== config file is marked config"
case "$kind" in
  deb)
    dpkg-query -W -f '${Conffiles}\n' datasafes3 | grep -q '/etc/datasafes3/datasafes3.env'
    dpkg -s datasafes3 | grep -q "Version: ${version}-1"
    ;;
  rpm)
    rpm -qc datasafes3 | grep -qx '/etc/datasafes3/datasafes3.env'
    rpm -q datasafes3 | grep -q "datasafes3-${version}-1"
    ;;
esac

echo "== unit is valid and not enabled"
if ! systemd-analyze verify --man=no datasafes3.service; then
  echo "systemd-analyze verify failed" >&2
  exit 1
fi
test ! -e /etc/systemd/system/multi-user.target.wants/datasafes3.service
test ! -e /etc/systemd/system/datasafes3.service
if command -v systemctl >/dev/null 2>&1; then
  if systemctl is-active --quiet datasafes3.service 2>/dev/null; then
    echo "datasafes3.service is active after install" >&2
    exit 1
  fi
fi

echo "== packaged env refuses the historical secrets"
set +e
runuser -u datasafes3 -- /bin/sh -c '
  set -a
  . /etc/datasafes3/datasafes3.env
  set +a
  exec /usr/bin/datasafe-storage-server
' >/tmp/datasafes3-strict.log 2>&1
strict_code=$?
set -e
if [ "$strict_code" -eq 0 ]; then
  echo "server started with the packaged env; strict secrets should refuse it" >&2
  cat /tmp/datasafes3-strict.log >&2 || true
  exit 1
fi
grep -q 'refusing to start' /tmp/datasafes3-strict.log

echo "== binary runs"
runuser -u datasafes3 -- env \
  STORAGE_DEV=true \
  STORAGE_ADDR=127.0.0.1:9000 \
  STORAGE_DATA_DIR=/var/lib/datasafes3 \
  STORAGE_STRICT_SECRETS=false \
  /usr/bin/datasafe-storage-server >/tmp/datasafes3.log 2>&1 &
pid=$!
cleanup() {
  if [ -n "${pid:-}" ] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

ok=0
for _ in $(seq 1 40); do
  if curl -sf http://127.0.0.1:9000/healthz >/tmp/healthz.json; then
    ok=1
    break
  fi
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "server exited before healthz" >&2
    cat /tmp/datasafes3.log >&2 || true
    exit 1
  fi
  sleep 0.5
done
if [ "$ok" -ne 1 ]; then
  echo "healthz did not succeed" >&2
  cat /tmp/datasafes3.log >&2 || true
  exit 1
fi
grep -q "\"version\":\"${version}\"" /tmp/healthz.json
test -f /var/lib/datasafes3/metadata.db
owner="$(stat -c '%U' /var/lib/datasafes3/metadata.db)"
test "$owner" = "datasafes3"
storage-cli --help >/dev/null

cleanup
trap - EXIT
pid=""

echo "== upgrade keeps config edits"
marker="# datasafes3-operator-edit"
printf '\n%s\n' "$marker" >> /etc/datasafes3/datasafes3.env
case "$kind" in
  deb) apt-get install -y --reinstall "$pkg" ;;
  rpm) rpm -Uvh --replacepkgs "$pkg" ;;
esac
grep -qx "$marker" /etc/datasafes3/datasafes3.env

echo "== uninstall keeps data"
printf 'kept\n' > /var/lib/datasafes3/keep-me
chown datasafes3:datasafes3 /var/lib/datasafes3/keep-me
case "$kind" in
  deb)
    apt-get remove -y datasafes3
    test -f /var/lib/datasafes3/metadata.db
    test -f /var/lib/datasafes3/keep-me
    test ! -x /usr/bin/datasafe-storage-server
    apt-get purge -y datasafes3
    test -f /var/lib/datasafes3/metadata.db
    test -f /var/lib/datasafes3/keep-me
    ;;
  rpm)
    rpm -e datasafes3
    test -f /var/lib/datasafes3/metadata.db
    test -f /var/lib/datasafes3/keep-me
    test ! -x /usr/bin/datasafe-storage-server
    ;;
esac
test ! -x /usr/bin/storage-cli

echo "package install check passed ($kind $version)"
