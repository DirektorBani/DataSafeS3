#!/usr/bin/env bash
# Publish one DataSafeS3 release tag to GitVerse package registries.
# Docker: https://gitverse.ru/docs/artifactory/packages-uc/docker-packages-uc
# DEB:    https://gitverse.ru/docs/artifactory/packages-uc/deb-packages-uc
# RPM:    https://gitverse.ru/docs/artifactory/packages-uc/rpm-packages-uc/
# Helm:   https://gitverse.ru/docs/artifactory/packages-uc/helm-packages-uc/
#
# Do not enable xtrace: the GitVerse token is passed on stdin and via a
# mode-0600 netrc file, and must not appear in the log.
set -euo pipefail

: "${TAG:?TAG is required, for example v1.7.0}"
: "${GITVERSE_USER:?GITVERSE_USER is required}"
: "${GITVERSE_TOKEN:?GITVERSE_TOKEN is required}"
: "${PKG_DIR:?PKG_DIR is required}"
: "${CHART_SRC:?CHART_SRC is required}"

ARCHITECTURES="${ARCHITECTURES:-all}"
DELETE_OLD="${DELETE_OLD:-false}"
KEEP="${KEEP:-2}"
LIMIT_MB="${LIMIT_MB:-500}"
DEB_DISTRIBUTION="${GITVERSE_DEB_DISTRIBUTION:-stable}"
DEB_COMPONENT="${GITVERSE_DEB_COMPONENT:-main}"
RPM_GROUP="${GITVERSE_RPM_GROUP:-stable}"
DRY_RUN="${GITVERSE_DRY_RUN:-0}"
SERVER_SRC="${SERVER_SRC:-ghcr.io/direktorbani/datasafe-storage-server:${TAG}}"
CONSOLE_SRC="${CONSOLE_SRC:-ghcr.io/direktorbani/datasafe-console:${TAG}}"

case "$ARCHITECTURES" in
  all|amd64) ;;
  *) echo "ARCHITECTURES must be all or amd64, got: $ARCHITECTURES" >&2; exit 1 ;;
esac
case "$DELETE_OLD" in
  true|false) ;;
  *) echo "DELETE_OLD must be true or false, got: $DELETE_OLD" >&2; exit 1 ;;
esac
case "$KEEP" in
  ''|*[!0-9]*) echo "KEEP must be an integer >= 1" >&2; exit 1 ;;
esac
if [ "$KEEP" -lt 1 ]; then
  echo "KEEP must be >= 1" >&2
  exit 1
fi
case "$TAG" in
  v[0-9]*) ;;
  *) echo "TAG must look like v1.7.0, got: $TAG" >&2; exit 1 ;;
esac

VERSION="${TAG#v}"
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
OWNER_LC=$(printf '%s' "$GITVERSE_USER" | tr '[:upper:]' '[:lower:]')
SERVER_DST="gitverse.ru/${OWNER_LC}/datasafe-storage-server"
CONSOLE_DST="gitverse.ru/${OWNER_LC}/datasafe-console"
API="https://gitverse.ru/api/packages/${GITVERSE_USER}"
SUMMARY="${GITHUB_STEP_SUMMARY:-}"

if [ -z "${GH_TOKEN:-}" ] && [ -n "${GITHUB_TOKEN:-}" ]; then
  GH_TOKEN=$GITHUB_TOKEN
  export GH_TOKEN
fi
if [ -z "${GH_USERNAME:-}" ] && [ -n "${GITHUB_ACTOR:-}" ]; then
  GH_USERNAME=$GITHUB_ACTOR
  export GH_USERNAME
fi

chart_version=$(awk '/^version:/{print $2; exit}' "$CHART_SRC/Chart.yaml")
if [ "$chart_version" != "$VERSION" ]; then
  echo "Chart.yaml version ${chart_version} does not match tag version ${VERSION}" >&2
  exit 1
fi

if [ "$DRY_RUN" != "1" ]; then
  command -v crane >/dev/null || { echo "crane is not installed" >&2; exit 1; }
  command -v helm >/dev/null || { echo "helm is not installed" >&2; exit 1; }
  command -v python3 >/dev/null || { echo "python3 is not installed" >&2; exit 1; }
  helm package "$CHART_SRC" --destination "$PKG_DIR"
fi

shopt -s nullglob
deb_amd64=("${PKG_DIR}"/datasafes3_*_amd64.deb)
deb_arm64=("${PKG_DIR}"/datasafes3_*_arm64.deb)
rpm_amd64=("${PKG_DIR}"/datasafes3-*.x86_64.rpm)
rpm_arm64=("${PKG_DIR}"/datasafes3-*.aarch64.rpm)
charts=("${PKG_DIR}"/datasafes3-*.tgz)
shopt -u nullglob

expect_one() {
  local label="$1"
  shift
  if [ "$#" -ne 1 ]; then
    echo "expected one ${label}, found $# (${*:-none})" >&2
    exit 1
  fi
}

expect_one "amd64 deb" "${deb_amd64[@]}"
expect_one "x86_64 rpm" "${rpm_amd64[@]}"
expect_one "helm chart" "${charts[@]}"
if [ "$ARCHITECTURES" = "all" ]; then
  expect_one "arm64 deb" "${deb_arm64[@]}"
  expect_one "aarch64 rpm" "${rpm_arm64[@]}"
fi

debs=("${deb_amd64[@]}")
rpms=("${rpm_amd64[@]}")
if [ "$ARCHITECTURES" = "all" ]; then
  debs+=("${deb_arm64[@]}")
  rpms+=("${rpm_arm64[@]}")
fi

run() {
  if [ "$DRY_RUN" = "1" ]; then
    printf 'dry-run:'
    printf ' %q' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

measure() {
  python3 "$SCRIPT_DIR/gitverse_publish.py" measure \
    --image "$SERVER_SRC" \
    --image "$CONSOLE_SRC" \
    --packages-dir "$PKG_DIR" \
    --architectures "$ARCHITECTURES" \
    --limit-mb "$LIMIT_MB" \
    --keep "$KEEP" \
    --summary "$SUMMARY"
}

prune() {
  local keep_non_alias="$1"
  python3 "$SCRIPT_DIR/gitverse_publish.py" prune \
    --user "$GITVERSE_USER" \
    --keep "$keep_non_alias" \
    --package "container/datasafe-storage-server" \
    --package "container/datasafe-console" \
    --package "debian/datasafes3" \
    --package "rpm/datasafes3" \
    --package "helm/datasafes3"
}

if [ "$DRY_RUN" = "1" ]; then
  echo "dry-run: would measure images and packages, then refuse the upload when over the limit"
else
  set +e
  measure
  measure_rc=$?
  set -e
  if [ "$measure_rc" -ne 0 ]; then
    echo "size check failed (exit ${measure_rc}); nothing was uploaded" >&2
    exit "$measure_rc"
  fi
  if [ "$DELETE_OLD" = "true" ]; then
    # Free a slot for this release before the upload. Alias tag "latest" is kept.
    prune "$((KEEP - 1))"
  fi
fi

copy_image() {
  local src="$1" dst_repo="$2"
  local version_ref="${dst_repo}:${TAG}" latest_ref="${dst_repo}:latest"
  if [ "$ARCHITECTURES" = "amd64" ]; then
    # crane copy --platform keeps a single-arch image (global flag, default is all).
    run crane copy --platform linux/amd64 "$src" "$version_ref"
  else
    # Drop BuildKit attestation manifests (platform unknown/unknown). The child
    # image manifests for linux/amd64 and linux/arm64 are copied by digest.
    # crane's own example: "Filter out weird platforms ... copy result".
    run crane index filter "$src" \
      --platform linux/amd64 \
      --platform linux/arm64 \
      --tag "$version_ref"
  fi
  run crane copy "$version_ref" "$latest_ref"
}

upload_file() {
  local method="$1" file="$2" url="$3"
  if [ "$DRY_RUN" = "1" ]; then
    printf 'dry-run: curl -X %s --upload-file %q %q\n' "$method" "$file" "$url"
    return 0
  fi
  local body code
  body=$(mktemp)
  code=$(curl --silent --show-error \
    --netrc-file "$netrc" \
    --retry 3 --retry-delay 2 \
    -X "$method" \
    --upload-file "$file" \
    --output "$body" \
    --write-out '%{http_code}' \
    "$url") || code="000"
  if [ "$code" = "200" ] || [ "$code" = "201" ] || [ "$code" = "204" ]; then
    echo "uploaded $(basename "$file") HTTP ${code}"
    rm -f "$body"
    return 0
  fi
  if [ "$code" = "409" ]; then
    echo "already published, left in place: $(basename "$file") HTTP 409"
    rm -f "$body"
    return 0
  fi
  echo "upload failed: $(basename "$file") HTTP ${code}" >&2
  echo "url: ${url}" >&2
  cat "$body" >&2 || true
  rm -f "$body"
  return 1
}

verify_ref() {
  local ref="$1"
  if [ "$DRY_RUN" = "1" ]; then
    echo "dry-run: would verify manifest ${ref} for ${ARCHITECTURES}"
    return 0
  fi
  crane manifest "$ref" | python3 "$SCRIPT_DIR/gitverse_publish.py" verify \
    --manifest-file - \
    --architectures "$ARCHITECTURES"
}

netrc=""
cleanup() {
  if [ -n "$netrc" ]; then
    rm -f "$netrc"
  fi
}
trap cleanup EXIT

if [ "$DRY_RUN" != "1" ]; then
  : "${GITHUB_TOKEN:?GITHUB_TOKEN is required to read ghcr.io}"
  : "${GITHUB_ACTOR:?GITHUB_ACTOR is required to read ghcr.io}"
  umask 077
  netrc=$(mktemp)
  printf 'machine gitverse.ru login %s password %s\n' "$GITVERSE_USER" "$GITVERSE_TOKEN" > "$netrc"
  printf '%s\n' "$GITHUB_TOKEN" | crane auth login ghcr.io -u "$GITHUB_ACTOR" --password-stdin
  printf '%s\n' "$GITVERSE_TOKEN" | crane auth login gitverse.ru -u "$GITVERSE_USER" --password-stdin
fi

copy_image "$SERVER_SRC" "$SERVER_DST"
copy_image "$CONSOLE_SRC" "$CONSOLE_DST"
verify_ref "${SERVER_DST}:${TAG}"
verify_ref "${CONSOLE_DST}:${TAG}"

deb_url="${API}/debian/pool/${DEB_DISTRIBUTION}/${DEB_COMPONENT}/upload"
rpm_url="${API}/rpm/${RPM_GROUP}/upload"
helm_url="${API}/helm/api/charts"
for file in "${debs[@]}"; do
  upload_file PUT "$file" "$deb_url"
done
for file in "${rpms[@]}"; do
  upload_file PUT "$file" "$rpm_url"
done
upload_file POST "${charts[0]}" "$helm_url"

if [ "$DRY_RUN" != "1" ] && [ "$DELETE_OLD" = "true" ]; then
  prune "$KEEP"
fi

note=$(printf 'Published %s to gitverse.ru/%s (%s).\ndocker pull %s:%s\ndocker pull %s:%s\n' \
  "$TAG" "$OWNER_LC" "$ARCHITECTURES" "$SERVER_DST" "$TAG" "$CONSOLE_DST" "$TAG")
printf '%s\n' "$note"
if [ -n "$SUMMARY" ]; then
  printf '%s\n' "$note" >> "$SUMMARY"
fi
