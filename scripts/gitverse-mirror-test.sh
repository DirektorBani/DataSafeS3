#!/usr/bin/env bash
# Local checks for scripts/gitverse-mirror.sh. No network and no real token.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
SCRIPT="$ROOT/scripts/gitverse-mirror.sh"
TOKEN='super-secret-token-value-xyz'
FAKE_KEY='-----BEGIN OPENSSH PRIVATE KEY-----
not-a-real-key
-----END OPENSSH PRIVATE KEY-----'

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

git init -q -b main "$TMP/src"
git -C "$TMP/src" config user.email test@example.com
git -C "$TMP/src" config user.name test
echo one > "$TMP/src/README"
git -C "$TMP/src" add README
git -C "$TMP/src" commit -q -m one
git -C "$TMP/src" tag v1.0.0
git -C "$TMP/src" checkout -q -b feature
echo feature > "$TMP/src/FEATURE"
git -C "$TMP/src" add FEATURE
git -C "$TMP/src" commit -q -m feature
git -C "$TMP/src" checkout -q main

git init -q --bare -b main "$TMP/dest.git"
git clone -q "$TMP/src" "$TMP/work"

run_mirror() {
  local rc=0
  set +e
  OUT=$(
    GITVERSE_TOKEN="${GITVERSE_TOKEN:-}" \
    GITVERSE_SSH_KEY="${GITVERSE_SSH_KEY:-}" \
    GITVERSE_MIRROR_URL="${GITVERSE_MIRROR_URL:-}" \
    GITVERSE_MIRROR_AUTH="${GITVERSE_MIRROR_AUTH:-https}" \
    GITVERSE_MIRROR_FORCE="${GITVERSE_MIRROR_FORCE:-}" \
    GITVERSE_MIRROR_DRY_RUN="${GITVERSE_MIRROR_DRY_RUN:-}" \
    GITVERSE_MIRROR_ASKPASS_COPY="${GITVERSE_MIRROR_ASKPASS_COPY:-}" \
    EVENT_NAME="${EVENT_NAME:-}" \
    INPUT_FORCE="${INPUT_FORCE:-}" \
    VAR_FORCE="${VAR_FORCE:-}" \
    GITHUB_ACTIONS='' \
    GITHUB_STEP_SUMMARY='' \
    GITVERSE_MIRROR_WORKDIR="$TMP/work" \
    bash "$SCRIPT" 2>&1
  )
  rc=$?
  set -e
  printf '%s\n' "$OUT" > "$TMP/last-out.txt"
  return "$rc"
}

assert_no_secret() {
  if grep -F "$TOKEN" "$TMP/last-out.txt" >/dev/null; then
    echo "token appeared in mirror output" >&2
    exit 1
  fi
  if grep -F "not-a-real-key" "$TMP/last-out.txt" >/dev/null; then
    echo "ssh key appeared in mirror output" >&2
    exit 1
  fi
}

echo "skip when no credentials"
if ! GITVERSE_TOKEN='' GITVERSE_SSH_KEY='' GITVERSE_MIRROR_URL="$TMP/dest.git" run_mirror; then
  echo "skip path must exit 0" >&2
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
grep -F "Mirror skipped" "$TMP/last-out.txt" >/dev/null
if git --git-dir="$TMP/dest.git" show-ref | grep -q .; then
  echo "skip path must not push" >&2
  exit 1
fi

echo "mirror branches and tags"
ASK="$TMP/askpass"
if ! GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" GITVERSE_MIRROR_ASKPASS_COPY="$ASK" run_mirror; then
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
assert_no_secret
if grep -F "super-secret" "$ASK" >/dev/null; then
  echo "askpass file contains the token" >&2
  exit 1
fi
user_out=$(GITVERSE_USER=DirektorBani GITVERSE_TOKEN="$TOKEN" "$ASK" "Username for 'https://gitverse.ru': ")
[ "$user_out" = "DirektorBani" ]
pass_out=$(GITVERSE_USER=DirektorBani GITVERSE_TOKEN="$TOKEN" "$ASK" "Password for 'https://gitverse.ru': ")
[ "$pass_out" = "$TOKEN" ]
git --git-dir="$TMP/dest.git" rev-parse --verify refs/heads/main >/dev/null
git --git-dir="$TMP/dest.git" rev-parse --verify refs/heads/feature >/dev/null
git --git-dir="$TMP/dest.git" rev-parse --verify refs/tags/v1.0.0 >/dev/null

echo "foreign branch stays"
git clone -q "$TMP/dest.git" "$TMP/foreign"
git -C "$TMP/foreign" config user.email test@example.com
git -C "$TMP/foreign" config user.name test
git -C "$TMP/foreign" checkout -q -b only-on-gitverse
echo foreign > "$TMP/foreign/FOREIGN"
git -C "$TMP/foreign" add FOREIGN
git -C "$TMP/foreign" commit -q -m foreign
git -C "$TMP/foreign" push -q origin only-on-gitverse
if ! GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" run_mirror; then
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
assert_no_secret
git --git-dir="$TMP/dest.git" rev-parse --verify refs/heads/only-on-gitverse >/dev/null

echo "diverged branch is rejected without force; a new branch still updates"
git -C "$TMP/src" checkout -q -b topic
echo topic > "$TMP/src/TOPIC"
git -C "$TMP/src" add TOPIC
git -C "$TMP/src" commit -q -m topic
git -C "$TMP/src" checkout -q main
echo diverge-src > "$TMP/src/README"
git -C "$TMP/src" add README
git -C "$TMP/src" commit -q -m diverge-src
git clone -q "$TMP/dest.git" "$TMP/div"
git -C "$TMP/div" config user.email test@example.com
git -C "$TMP/div" config user.name test
echo diverge-dst > "$TMP/div/README"
git -C "$TMP/div" add README
git -C "$TMP/div" commit -q -m diverge-dst
git -C "$TMP/div" push -q origin main
BEFORE=$(git --git-dir="$TMP/dest.git" rev-parse refs/heads/main)
if GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" run_mirror; then
  echo "diverged push must fail" >&2
  exit 1
fi
assert_no_secret
grep -F "not a fast-forward" "$TMP/last-out.txt" >/dev/null
AFTER=$(git --git-dir="$TMP/dest.git" rev-parse refs/heads/main)
[ "$BEFORE" = "$AFTER" ]
git --git-dir="$TMP/dest.git" rev-parse --verify refs/heads/topic >/dev/null

echo "dry-run does not move the diverged branch"
if ! GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" GITVERSE_MIRROR_DRY_RUN=1 run_mirror; then
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
assert_no_secret
grep -F "would push" "$TMP/last-out.txt" >/dev/null
STILL=$(git --git-dir="$TMP/dest.git" rev-parse refs/heads/main)
[ "$STILL" = "$BEFORE" ]

echo "force updates the diverged branch and a moved tag"
SRC_MAIN=$(git -C "$TMP/src" rev-parse main)
git -C "$TMP/src" tag -f v1.0.0 "$SRC_MAIN"
if ! GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" GITVERSE_MIRROR_FORCE=true run_mirror; then
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
assert_no_secret
[ "$(git --git-dir="$TMP/dest.git" rev-parse refs/heads/main)" = "$SRC_MAIN" ]
[ "$(git --git-dir="$TMP/dest.git" rev-parse refs/tags/v1.0.0^{})" = "$SRC_MAIN" ]
git --git-dir="$TMP/dest.git" rev-parse --verify refs/heads/only-on-gitverse >/dev/null

echo "deleted GitHub branch remains on GitVerse"
git -C "$TMP/src" branch -D topic
if ! GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" run_mirror; then
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
git --git-dir="$TMP/dest.git" rev-parse --verify refs/heads/topic >/dev/null

echo "ssh auth without a key falls back to https"
if ! GITVERSE_TOKEN="$TOKEN" GITVERSE_SSH_KEY='' GITVERSE_MIRROR_AUTH=ssh GITVERSE_MIRROR_URL="$TMP/dest.git" run_mirror; then
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
assert_no_secret
grep -F "Using HTTPS" "$TMP/last-out.txt" >/dev/null

echo "https without a token uses the ssh key for a local url"
if ! GITVERSE_TOKEN='' GITVERSE_SSH_KEY="$FAKE_KEY" GITVERSE_MIRROR_AUTH=https GITVERSE_MIRROR_URL="$TMP/dest.git" run_mirror; then
  cat "$TMP/last-out.txt" >&2
  exit 1
fi
assert_no_secret
grep -F "Using the SSH key" "$TMP/last-out.txt" >/dev/null

echo "workflow_dispatch force=false does not honor a true repository variable"
echo ahead > "$TMP/src/README"
git -C "$TMP/src" add README
git -C "$TMP/src" commit -q -m ahead
WANTED=$(git -C "$TMP/src" rev-parse main)
git clone -q "$TMP/dest.git" "$TMP/div2"
git -C "$TMP/div2" config user.email test@example.com
git -C "$TMP/div2" config user.name test
echo other > "$TMP/div2/README"
git -C "$TMP/div2" add README
git -C "$TMP/div2" commit -q -m other
git -C "$TMP/div2" push -q origin main
if EVENT_NAME=workflow_dispatch INPUT_FORCE=false VAR_FORCE=true \
  GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" run_mirror; then
  echo "dispatch force=false must not force-push" >&2
  exit 1
fi
assert_no_secret
[ "$(git --git-dir="$TMP/dest.git" rev-parse refs/heads/main)" != "$WANTED" ]

echo "bad force value fails"
if GITVERSE_TOKEN="$TOKEN" GITVERSE_MIRROR_URL="$TMP/dest.git" GITVERSE_MIRROR_FORCE=maybe run_mirror; then
  echo "bad force value must fail" >&2
  exit 1
fi
assert_no_secret

echo "gitverse-mirror-test ok"
