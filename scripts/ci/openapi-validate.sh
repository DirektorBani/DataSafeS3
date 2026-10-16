#!/bin/sh
# Fail unless api/openapi/v1.yaml matches the OpenAPI 3.1.0 meta-schema.
# The three checks at the end must be rejected: license identifier+url,
# a response with no description, and an unquoted flow value that contains
# commas (YAML splits it into extra keys).
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"

schema=$root/api/openapi/oas-3.1.schema.json
spec=$root/api/openapi/v1.yaml
tool=$root/tools/openapi31validate

check() {
  ( cd "$tool" && go run . "$schema" "$1" )
}

echo "==> OpenAPI 3.1 schema"
check "$spec"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

expect_fail() {
  name=$1
  file=$2
  needle=$3
  set +e
  check "$file" >"$tmp/out" 2>&1
  code=$?
  set -e
  if [ "$code" -eq 0 ]; then
    echo "self-test $name: validator accepted an invalid document" >&2
    cat "$tmp/out" >&2
    exit 1
  fi
  if [ "$code" -ne 1 ]; then
    echo "self-test $name: validator failed to run (exit $code)" >&2
    cat "$tmp/out" >&2
    exit 1
  fi
  if ! grep -q "$needle" "$tmp/out"; then
    echo "self-test $name: rejection did not mention $needle" >&2
    cat "$tmp/out" >&2
    exit 1
  fi
  echo "self-test $name: rejected"
}

python3 - "$spec" "$tmp/both-license.yaml" << 'PY'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
old = "    identifier: Apache-2.0\n"
new = old + "    url: https://www.apache.org/licenses/LICENSE-2.0\n"
if old not in src:
    raise SystemExit("license identifier line missing")
pathlib.Path(sys.argv[2]).write_text(src.replace(old, new, 1))
PY
expect_fail license "$tmp/both-license.yaml" "/info/license"

python3 - "$spec" "$tmp/no-description.yaml" << 'PY'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
old = "        '200': { description: Encryption view }\n"
new = "        '200': {}\n"
if old not in src:
    raise SystemExit("encryption response line missing")
pathlib.Path(sys.argv[2]).write_text(src.replace(old, new, 1))
PY
expect_fail description "$tmp/no-description.yaml" "missing property 'description'"

python3 - "$spec" "$tmp/unquoted.yaml" << 'PY'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
old = '        \'200\': { description: "Policy, effective policy, and locked fields" }\n'
new = "        '200': { description: Policy, effective policy, and locked fields }\n"
if old not in src:
    raise SystemExit("sharing description line missing")
pathlib.Path(sys.argv[2]).write_text(src.replace(old, new, 1))
PY
expect_fail "unquoted description" "$tmp/unquoted.yaml" "effective policy"
