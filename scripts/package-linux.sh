#!/usr/bin/env bash
# Build datasafes3 .deb and .rpm packages for linux/amd64 and linux/arm64.
# Version comes from VERSION. Output is dist/packages/ plus SHA256SUMS.txt.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NFPM_VERSION=2.48.0

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

# nfpm is built from source through the GitVerse Go mirror (no download from
# a foreign host). GOPROXY can be overridden, e.g. for an offline mirror.
install_nfpm() {
  if [ -x "$TOOLS/nfpm" ] && "$TOOLS/nfpm" --version 2>/dev/null | grep -q "$NFPM_VERSION"; then
    return
  fi
  GOBIN="$TOOLS" GOPROXY="${GOPROXY:-https://go-mirror.gitverse.ru,direct}" \
    go install "github.com/goreleaser/nfpm/v2/cmd/nfpm@v${NFPM_VERSION}"
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
  sha256sum datasafes3_"${VERSION}"_*.deb datasafes3-"${VERSION}"-1.*.rpm | sort -k2 > SHA256SUMS.txt
)

echo "packages in $OUT"
ls -l "$OUT"
