#!/usr/bin/env bash
# Build datasafes3 .deb and .rpm packages for linux/amd64 and linux/arm64.
# Version comes from VERSION. Output is dist/packages/ plus SHA256SUMS.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NFPM_VERSION=2.48.0
# Published checksums for the nfpm release tarballs.
NFPM_SHA_X86_64=6d7f9a28b2f397934403950fcbba83f598b08d02edbe79a0815b3b4897341750
NFPM_SHA_ARM64=a9a75213763783b7669719e505474c748c5ad7dc66591a195619deb50f85c2f4

VERSION="$(tr -d '[:space:]' < VERSION)"
if ! printf '%s' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "package-linux: VERSION must be X.Y.Z, got ${VERSION}" >&2
  exit 1
fi

COMMIT="${GIT_COMMIT:-}"
if [ -z "$COMMIT" ]; then
  COMMIT="$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
fi

OUT="${OUT:-$ROOT/dist/packages}"
TOOLS="$ROOT/dist/tools"
mkdir -p "$OUT" "$TOOLS"

install_nfpm() {
  local os arch asset expect tarball
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Linux) ;;
    *)
      echo "package-linux: nfpm install supports Linux (got $os)" >&2
      exit 1
      ;;
  esac
  case "$arch" in
    x86_64)
      asset="nfpm_${NFPM_VERSION}_Linux_x86_64.tar.gz"
      expect="$NFPM_SHA_X86_64"
      ;;
    aarch64|arm64)
      asset="nfpm_${NFPM_VERSION}_Linux_arm64.tar.gz"
      expect="$NFPM_SHA_ARM64"
      ;;
    *)
      echo "package-linux: unsupported arch $arch" >&2
      exit 1
      ;;
  esac
  if [ -x "$TOOLS/nfpm" ] && "$TOOLS/nfpm" --version 2>/dev/null | grep -q "$NFPM_VERSION"; then
    return
  fi
  tarball="$TOOLS/$asset"
  curl -fsSL -o "$tarball" "https://github.com/goreleaser/nfpm/releases/download/v${NFPM_VERSION}/${asset}"
  echo "${expect}  ${tarball}" | sha256sum -c -
  tar -xzf "$tarball" -C "$TOOLS" nfpm
  chmod 0755 "$TOOLS/nfpm"
}

install_nfpm
NFPM="$TOOLS/nfpm"

build_arch() {
  local goarch="$1"
  local nfpmarch="$2"
  local rpmarch="$3"
  local bindir="$ROOT/dist/build/linux_${goarch}"
  mkdir -p "$bindir"
  local ldflags="-s -w -X github.com/DirektorBani/datasafe/internal/buildinfo.Version=${VERSION} -X github.com/DirektorBani/datasafe/internal/buildinfo.Commit=${COMMIT}"
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -trimpath -tags datasafe_sinkexec \
    -ldflags "$ldflags" \
    -o "$bindir/datasafe-storage-server" ./cmd/storage-server
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -trimpath -ldflags "-s -w" \
    -o "$bindir/storage-cli" ./cmd/storage-cli
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -trimpath -ldflags "-s -w" \
    -o "$bindir/datasafe-kafka-bridge" ./cmd/datasafe-kafka-bridge

  VERSION="$VERSION" ARCH="$nfpmarch" BINDIR="$bindir" \
    "$NFPM" pkg -f packaging/nfpm.yaml -p deb \
    -t "$OUT/datasafes3_${VERSION}_${nfpmarch}.deb"
  VERSION="$VERSION" ARCH="$nfpmarch" BINDIR="$bindir" \
    "$NFPM" pkg -f packaging/nfpm.yaml -p rpm \
    -t "$OUT/datasafes3-${VERSION}-1.${rpmarch}.rpm"
}

build_arch amd64 amd64 x86_64
build_arch arm64 arm64 aarch64

(
  cd "$OUT"
  sha256sum datasafes3_"${VERSION}"_*.deb datasafes3-"${VERSION}"-1.*.rpm | sort -k2 > SHA256SUMS
)

echo "packages in $OUT"
ls -l "$OUT"
