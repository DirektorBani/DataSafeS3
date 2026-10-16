#!/bin/sh
# Fail when api/openapi/v1.yaml breaks the last released contract.
# Until a v1.* tag contains that file, the comparison is skipped and the
# control checks below still run: removing a required field fails, adding an
# optional field passes.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"
spec=api/openapi/v1.yaml

oasdiff=${OASDIFF:-}
if [ -z "$oasdiff" ]; then
  if command -v oasdiff >/dev/null 2>&1; then
    oasdiff=oasdiff
  elif [ -x "$(go env GOPATH)/bin/oasdiff" ]; then
    oasdiff="$(go env GOPATH)/bin/oasdiff"
  else
    go install github.com/oasdiff/oasdiff@v1.15.0
    oasdiff="$(go env GOPATH)/bin/oasdiff"
  fi
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

python3 - "$spec" "$tmp" << 'PY'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
prop = (
    "        login_methods:\n"
    "          type: array\n"
    "          items: { type: string, enum: [password, oidc] }\n"
)
removed = src.replace(
    "      required: [api, server_version, spec_version, login_methods]\n",
    "      required: [api, server_version, spec_version]\n",
    1,
).replace(prop, "", 1)
if removed == src:
    raise SystemExit("self-test could not remove login_methods")
pathlib.Path(sys.argv[2], "removed.yaml").write_text(removed)
added = src.replace("        login_methods:\n", "        trace_id: { type: string }\n        login_methods:\n", 1)
if added == src:
    raise SystemExit("self-test could not find login_methods")
pathlib.Path(sys.argv[2], "added.yaml").write_text(added)
PY

echo "self-test: removing required login_methods must fail"
set +e
"$oasdiff" breaking "$spec" "$tmp/removed.yaml" --fail-on ERR >"$tmp/removed.txt" 2>&1
rc=$?
set -e
if [ "$rc" -eq 0 ]; then
  echo "oasdiff accepted a removed required response field"
  cat "$tmp/removed.txt"
  exit 1
fi
if ! grep -q "error" "$tmp/removed.txt"; then
  echo "oasdiff exited $rc without an error finding"
  cat "$tmp/removed.txt"
  exit 1
fi

echo "self-test: adding an optional field must pass"
"$oasdiff" breaking "$spec" "$tmp/added.yaml" --fail-on ERR

if [ "${API_BREAKING_APPROVED:-}" = "true" ]; then
  echo "api-breaking-approved: comparison against the previous contract skipped"
  exit 0
fi

base=""
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  for tag in $(git tag -l 'v1.*' --sort=-v:refname); do
    if git cat-file -e "$tag:$spec" 2>/dev/null; then
      git show "$tag:$spec" >"$tmp/base.yaml"
      base="$tmp/base.yaml"
      echo "baseline: $tag"
      break
    fi
  done
  if [ -z "$base" ] && git rev-parse --verify origin/main >/dev/null 2>&1; then
    mb=$(git merge-base HEAD origin/main 2>/dev/null || true)
    if [ -n "$mb" ] && git cat-file -e "$mb:$spec" 2>/dev/null; then
      git show "$mb:$spec" >"$tmp/base.yaml"
      base="$tmp/base.yaml"
      echo "baseline: merge-base $mb"
    fi
  fi
fi

if [ -z "$base" ]; then
  echo "no v1.* tag or main merge-base contains $spec; breaking comparison skipped"
  exit 0
fi

"$oasdiff" breaking "$base" "$spec" --fail-on ERR
