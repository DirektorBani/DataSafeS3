#!/usr/bin/env bash
# Build is separate (scripts/package-linux.sh). This installs the packages
# in clean Debian and Rocky containers and checks uninstall keeps data.
# Set PACKAGE_TEST_SKIP_ARM64=1 to skip the emulated arm64 installs.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="${1:-$ROOT/dist/packages}"
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"

if ! command -v docker >/dev/null 2>&1; then
  echo "test-linux-packages: docker is required" >&2
  exit 1
fi

run_one() {
  local image="$1"
  local platform="$2"
  local pkg="$3"
  local kind="$4"
  local name
  name="$(basename "$pkg")"
  echo "---- $image $platform $name"
  docker run --rm --platform "$platform" \
    -v "$DIR:/packages:ro" \
    -v "$ROOT/scripts/package-install-check.sh:/check.sh:ro" \
    "$image" \
    bash /check.sh "/packages/$name" "$kind" "$VERSION"
}

run_one debian:12 linux/amd64 "$DIR/datasafes3_${VERSION}_amd64.deb" deb
run_one rockylinux:9 linux/amd64 "$DIR/datasafes3-${VERSION}-1.x86_64.rpm" rpm

if [ "${PACKAGE_TEST_SKIP_ARM64:-}" = "1" ]; then
  echo "skipping arm64 package install checks"
  exit 0
fi

run_one debian:12 linux/arm64 "$DIR/datasafes3_${VERSION}_arm64.deb" deb
run_one rockylinux:9 linux/arm64 "$DIR/datasafes3-${VERSION}-1.aarch64.rpm" rpm

echo "linux package install checks passed"
