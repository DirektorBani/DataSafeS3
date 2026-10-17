#!/bin/sh
# Run record.sh from Git bash and fail if a temp file is left behind.
# Usage: desktop-record-gitbash.sh <directory with record.sh> <base-url> [version]
set -eu
dir="${1:?directory with record.sh}"
base="${2:?base url}"
version="${3:-1.6.0}"
cd "$dir"
scratch=$(mktemp -d)
cleanup() {
  rm -rf "$scratch"
}
trap cleanup EXIT
TMPDIR="$scratch" bash ./record.sh "$version" "$base"
left=$(find "$scratch" -type f | wc -l | tr -d '[:space:]')
if [ "$left" != "0" ]; then
  echo "record.sh left $left temp file(s)" >&2
  find "$scratch" -type f >&2
  exit 1
fi
