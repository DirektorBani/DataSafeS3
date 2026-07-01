#!/usr/bin/env sh
# Extract a single ## [version] section from CHANGELOG.md for GitHub Release body_path.
# Usage: extract-changelog-section.sh 1.0.3 [CHANGELOG.md] [release-body.md]
set -eu
VERSION="${1:?version required (e.g. 1.0.3)}"
CHANGELOG="${2:-CHANGELOG.md}"
OUT="${3:-release-body.md}"
awk -v ver="$VERSION" '
  $0 ~ "^## \\[" ver "\\]" { found=1; print; next }
  found && /^## \[/ { exit }
  found { print }
' "$CHANGELOG" > "$OUT"
if ! grep -q "^\## \[$VERSION\]" "$OUT" 2>/dev/null; then
  echo "extract-changelog-section: section ## [$VERSION] not found in $CHANGELOG" >&2
  exit 1
fi
echo "Wrote $OUT"
