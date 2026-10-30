#!/usr/bin/env bash
# Build and push the DataSafeS3 images (storage server and console) for
# linux/amd64 and linux/arm64 WITHOUT a Docker daemon, kaniko or QEMU.
#
# Go cross-compiles the binaries, npm builds the console, and tools/imgbuild
# appends them as layers to the Alpine base image and pushes the result. The
# image content is the same as deploy/docker/Dockerfile and Dockerfile.console
# (their final stages are COPY only).
#
# Everything is fetched from GitVerse mirrors by default, so the build does not
# depend on Docker Hub, proxy.golang.org or registry.npmjs.org:
#   BASE_REGISTRY  dh-mirror.gitverse.ru/library/   (Docker Hub mirror)
#   GOPROXY        https://go-mirror.gitverse.ru,direct
#   NPM_REGISTRY   https://npm-mirror.gitverse.ru/
#
# Usage:
#   scripts/build-images.sh [--registry gitverse.ru/direktorbani] [--tags v1.7.1,latest]
#                           [--platforms linux/amd64,linux/arm64] [--skip-console-build]
# Registry login: DOCKER_CONFIG (for example `crane auth login gitverse.ru`).
# Do not enable xtrace.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

REGISTRY="gitverse.ru/direktorbani"
PLATFORMS="linux/amd64,linux/arm64"
VERSION="$(tr -d '[:space:]' < VERSION)"
TAGS="v${VERSION},latest"
SKIP_CONSOLE_BUILD=0
BASE_REGISTRY="${BASE_REGISTRY-dh-mirror.gitverse.ru/library/}"
ALPINE_VERSION="${ALPINE_VERSION:-3.24}"
export GOPROXY="${GOPROXY:-https://go-mirror.gitverse.ru,direct}"
NPM_REGISTRY="${NPM_REGISTRY-https://npm-mirror.gitverse.ru/}"
COMMIT="${GIT_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo unknown)}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct 2>/dev/null || date +%s)}"
export SOURCE_DATE_EPOCH

while [ $# -gt 0 ]; do
  case "$1" in
    --registry) REGISTRY="$2"; shift 2 ;;
    --tags) TAGS="$2"; shift 2 ;;
    --platforms) PLATFORMS="$2"; shift 2 ;;
    --skip-console-build) SKIP_CONSOLE_BUILD=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

OUT="$ROOT/dist/image"
rm -rf "$OUT"
mkdir -p "$OUT" "$ROOT/dist/tools"

echo "== imgbuild"
(cd tools/imgbuild && CGO_ENABLED=0 go build -trimpath -o "$ROOT/dist/tools/imgbuild" .)

echo "== storage-server binaries"
archs=()
IFS=, read -r -a plats <<< "$PLATFORMS"
for p in "${plats[@]}"; do
  arch="${p#linux/}"
  archs+=("$arch")
  mkdir -p "$OUT/linux_${arch}"
  TARGETARCH="$arch" VERSION="$VERSION" GIT_COMMIT="$COMMIT" OUT="$OUT/linux_${arch}" \
    sh deploy/docker/link-version.sh
done
tr -d '\015' < deploy/docker/docker-entrypoint.sh > "$OUT/docker-entrypoint.sh"
tr -d '\015' < deploy/docker/entrypoint-launcher.sh > "$OUT/datasafe-entrypoint"

if [ "$SKIP_CONSOLE_BUILD" != "1" ]; then
  echo "== console"
  (
    cd web/console
    # npm-mirror intermittently answers 504 for uncached tarballs: cap the mirror
    # attempt, then retry once against NPM_FALLBACK_REGISTRY (empty disables it).
    NPM_FALLBACK_REGISTRY="${NPM_FALLBACK_REGISTRY-https://registry.npmjs.org/}"
    if [ -n "$NPM_REGISTRY" ]; then
      if ! npm_config_registry="$NPM_REGISTRY" timeout 600 npm ci --no-audit --no-fund --fetch-retries=1 --fetch-timeout=60000; then
        [ -n "$NPM_FALLBACK_REGISTRY" ] || exit 1
        echo "npm ci via $NPM_REGISTRY failed, retrying via $NPM_FALLBACK_REGISTRY"
        npm_config_registry="$NPM_FALLBACK_REGISTRY" npm ci --no-audit --no-fund
      fi
    else
      npm ci
    fi
    npm run build
  )
fi
test -f web/console/dist/index.html

tag_args() {
  local repo="$1" t
  IFS=, read -r -a ts <<< "$TAGS"
  for t in "${ts[@]}"; do printf -- '--tag\n%s/%s:%s\n' "$REGISTRY" "$repo" "$t"; done
}
labels() {
  printf -- '--label\n%s\n' \
    "org.opencontainers.image.title=$1" \
    "org.opencontainers.image.version=${VERSION}" \
    "org.opencontainers.image.revision=${COMMIT}" \
    "org.opencontainers.image.source=https://gitverse.ru/DirektorBani/DataSafeS3" \
    "org.opencontainers.image.licenses=Apache-2.0"
}
BASE="${BASE_REGISTRY}alpine:${ALPINE_VERSION}"

echo "== datasafe-storage-server"
mapfile -t TA < <(tag_args datasafe-storage-server)
mapfile -t LA < <(labels datasafe-storage-server)
"$ROOT/dist/tools/imgbuild" --base "$BASE" --platform "$PLATFORMS" "${TA[@]}" "${LA[@]}" \
  --file "$OUT/linux_{arch}/storage-server=/storage-server:0755" \
  --file "$OUT/linux_{arch}/datasafe-kafka-bridge=/usr/lib/datasafe/datasafe-kafka-bridge:0755" \
  --file "$OUT/linux_{arch}/su-exec=/usr/local/bin/su-exec:0755" \
  --file "$OUT/docker-entrypoint.sh=/docker-entrypoint.sh:0755" \
  --file "$OUT/datasafe-entrypoint=/usr/local/bin/datasafe-entrypoint:0755" \
  --entrypoint /usr/local/bin/datasafe-entrypoint \
  --expose 9000/tcp \
  --healthcheck 'wget -q -O- http://127.0.0.1:9000/healthz || exit 1'

echo "== datasafe-console"
mapfile -t TA < <(tag_args datasafe-console)
mapfile -t LA < <(labels datasafe-console)
"$ROOT/dist/tools/imgbuild" --base "$BASE" --platform "$PLATFORMS" "${TA[@]}" "${LA[@]}" \
  --dir "$ROOT/web/console/dist=/srv/console"
