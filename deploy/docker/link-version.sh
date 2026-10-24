#!/bin/sh
# Link VERSION and the git commit into storage-server.
# Docker does not parse this file, so the shell expansions stay intact.
# The image starts the Kafka bridge only when STORAGE_KAFKA_BROKERS is set.
#
# GOARCH comes from TARGETARCH (set by buildx / kaniko --customPlatform) or
# GOARCH, default amd64. Go cross-compiles, so no emulation is needed.
# OUT is the output directory (default /out).
set -eu
ver="${VERSION:-dev}"
if [ "$ver" = "dev" ] && [ -f VERSION ]; then
  ver=$(tr -d '[:space:]' < VERSION)
fi
commit="${GIT_COMMIT:-unknown}"
if [ -z "$commit" ]; then
  commit=unknown
fi
arch="${TARGETARCH:-${GOARCH:-amd64}}"
out="${OUT:-/out}"
mkdir -p "$out"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -tags datasafe_sinkexec \
  -ldflags "-s -w -X github.com/DirektorBani/datasafe/internal/buildinfo.Version=${ver} -X github.com/DirektorBani/datasafe/internal/buildinfo.Commit=${commit}" \
  -o "$out/storage-server" ./cmd/storage-server
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags "-s -w" \
  -o "$out/datasafe-kafka-bridge" ./cmd/datasafe-kafka-bridge
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags "-s -w" \
  -o "$out/su-exec" ./cmd/su-exec
