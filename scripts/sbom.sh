#!/usr/bin/env bash
# Build the per-release SBOM set (CycloneDX 1.5 JSON) into $OUT (default dist/sbom).
#
#   scripts/sbom.sh [VERSION]
#
# Produces:
#   sbom-go-storage-server.cdx.json  cyclonedx-gomod: Go modules compiled into storage-server
#   sbom-go-tools.cdx.json           cyclonedx-gomod: cli/other binaries (if present)
#   sbom-console-npm.cdx.json        cyclonedx-npm: production npm dependencies of web/console
#   sbom-image-<name>.cdx.json       syft: the published image (only when IMAGE_REGISTRY_REF_* are set)
#   SHA256SUMS.txt
#   sbom-<VERSION>.tgz               everything above (GitVerse release assets cannot be bare .json)
#
# Env:
#   OUT                    output dir (default dist/sbom)
#   SBOM_SERVER_IMAGE      e.g. gitverse.ru/direktorbani/datasafe-storage-server:v1.7.1 (optional)
#   SBOM_CONSOLE_IMAGE     e.g. gitverse.ru/direktorbani/datasafe-console:v1.7.1 (optional)
# Tools: cyclonedx-gomod, cyclonedx-npm, syft (installed by .gitverse/workflows/release.yaml: pinned release binaries + npm-mirror).
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${1:-$(cat VERSION)}"
OUT="${OUT:-dist/sbom}"
mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"

need() { command -v "$1" >/dev/null || { echo "missing tool: $1" >&2; exit 1; }; }
need cyclonedx-gomod
need cyclonedx-npm

echo "== Go modules (storage-server)"
cyclonedx-gomod app -json -licenses -packages -main cmd/storage-server \
  -output "$OUT/sbom-go-storage-server.cdx.json" .

echo "== npm production dependencies (web/console)"
(cd web/console && cyclonedx-npm --omit dev --spec-version 1.5 --output-format JSON \
  --output-file "$OUT/sbom-console-npm.cdx.json")

if command -v syft >/dev/null; then
  for pair in "storage-server:${SBOM_SERVER_IMAGE:-}" "console:${SBOM_CONSOLE_IMAGE:-}"; do
    name="${pair%%:*}"; ref="${pair#*:}"
    [ -n "$ref" ] || continue
    echo "== image $ref"
    syft "registry:$ref" -o "cyclonedx-json=$OUT/sbom-image-$name.cdx.json" -q
  done
else
  echo "syft not installed: image SBOMs skipped" >&2
fi

(cd "$OUT" && sha256sum ./*.json | sed 's# \./# #' > SHA256SUMS.txt)
(cd "$OUT" && tar -czf "sbom-${VERSION}.tgz" ./*.json SHA256SUMS.txt)
echo "SBOM files in $OUT:"; ls -l "$OUT"
