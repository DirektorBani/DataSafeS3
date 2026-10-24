#!/usr/bin/env bash
# Upload the .deb/.rpm packages and the Helm chart of one release tag to the
# GitVerse package registries. Images are pushed by scripts/build-images.sh,
# not here (no ghcr.io copy: GitHub is not part of the release path).
#
#   TAG=v1.7.1 GV_USER=DirektorBani GV_TOKEN=... PKG_DIR=dist/packages \
#     CHART_SRC=deploy/helm/datasafes3 scripts/gitverse-publish-packages.sh
#
# 409 (already published) is treated as success so a re-run is safe.
# Do not enable xtrace: the token is passed through a mode-0600 netrc file.
set -euo pipefail

: "${TAG:?TAG is required, for example v1.7.1}"
: "${GV_USER:?GV_USER is required}"
: "${GV_TOKEN:?GV_TOKEN is required}"
: "${PKG_DIR:?PKG_DIR is required}"
: "${CHART_SRC:?CHART_SRC is required}"
DEB_DISTRIBUTION="${GITVERSE_DEB_DISTRIBUTION:-stable}"
DEB_COMPONENT="${GITVERSE_DEB_COMPONENT:-main}"
RPM_GROUP="${GITVERSE_RPM_GROUP:-stable}"
DRY_RUN="${GITVERSE_DRY_RUN:-0}"

case "$TAG" in v[0-9]*) ;; *) echo "TAG must look like v1.7.1, got: $TAG" >&2; exit 1 ;; esac
VERSION="${TAG#v}"
API="https://gitverse.ru/api/packages/${GV_USER}"

chart_version=$(awk '/^version:/{print $2; exit}' "$CHART_SRC/Chart.yaml")
if [ "$chart_version" != "$VERSION" ]; then
  echo "Chart.yaml version ${chart_version} does not match tag version ${VERSION}" >&2
  exit 1
fi
helm package "$CHART_SRC" --destination "$PKG_DIR"

shopt -s nullglob
debs=("$PKG_DIR"/datasafes3_"${VERSION}"_*.deb)
rpms=("$PKG_DIR"/datasafes3-"${VERSION}"-1.*.rpm)
charts=("$PKG_DIR"/datasafes3-"${VERSION}".tgz)
shopt -u nullglob
[ "${#debs[@]}" -eq 2 ] || { echo "expected 2 .deb, found ${#debs[@]}" >&2; exit 1; }
[ "${#rpms[@]}" -eq 2 ] || { echo "expected 2 .rpm, found ${#rpms[@]}" >&2; exit 1; }
[ "${#charts[@]}" -eq 1 ] || { echo "expected 1 chart tgz, found ${#charts[@]}" >&2; exit 1; }

umask 077
netrc=$(mktemp)
trap 'rm -f "$netrc"' EXIT
printf 'machine gitverse.ru login %s password %s\n' "$GV_USER" "$GV_TOKEN" > "$netrc"

upload_file() {
  local method="$1" file="$2" url="$3" body code
  if [ "$DRY_RUN" = "1" ]; then printf 'dry-run: %s %s -> %s\n' "$method" "$file" "$url"; return 0; fi
  body=$(mktemp)
  code=$(curl --silent --show-error --http1.1 --netrc-file "$netrc" --retry 5 --retry-delay 3 --retry-all-errors \
    -X "$method" --upload-file "$file" --output "$body" --write-out '%{http_code}' "$url") || code="000"
  case "$code" in
    200|201|204) echo "uploaded $(basename "$file") HTTP ${code}" ;;
    409) echo "already published, left in place: $(basename "$file") HTTP 409" ;;
    *) echo "upload failed: $(basename "$file") HTTP ${code} url=${url}" >&2; cat "$body" >&2 || true; rm -f "$body"; return 1 ;;
  esac
  rm -f "$body"
}

for f in "${debs[@]}"; do upload_file PUT "$f" "${API}/debian/pool/${DEB_DISTRIBUTION}/${DEB_COMPONENT}/upload"; done
for f in "${rpms[@]}"; do upload_file PUT "$f" "${API}/rpm/${RPM_GROUP}/upload"; done
upload_file POST "${charts[0]}" "${API}/helm/api/charts"
echo "packages published for ${TAG}"
