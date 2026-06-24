#!/usr/bin/env bash
# Build datasafe-sync sidecars for Tauri — run from repo root.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/clients/desktop/src-tauri/binaries"
mkdir -p "$OUT"

build() {
  local goos=$1 goarch=$2 name=$3
  echo "Building $name..."
  GOOS=$goos GOARCH=$goarch go build -o "$OUT/$name" "$ROOT/cmd/datasafe-sync"
}

build windows amd64 datasafe-sync-x86_64-pc-windows-msvc.exe
build linux amd64 datasafe-sync-x86_64-unknown-linux-gnu
build darwin amd64 datasafe-sync-x86_64-apple-darwin
build darwin arm64 datasafe-sync-aarch64-apple-darwin
echo "Sidecars in $OUT"
