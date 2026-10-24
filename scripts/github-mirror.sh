#!/usr/bin/env bash
# Push the current GitVerse ref (branch or tag) to the GitHub mirror.
#
# GitVerse is the primary repository, GitHub is a mirror. This is not
# git push --mirror and never forces: refs that exist only on GitHub (Cursor
# agent branches, refs/pull/*) stay in place, a non-fast-forward update is
# rejected. Needs GH_MIRROR_ACTOR, GH_MIRROR_REPO and GH_MIRROR_TOKEN
# (a GitHub token with repo and workflow scopes). The token is passed through
# GIT_ASKPASS, never put in the remote URL, and never printed.
# Usage: scripts/github-mirror.sh <refs/heads/x | refs/tags/y>
set -euo pipefail
unset GIT_TRACE GIT_TRACE_PACKET GIT_CURL_VERBOSE GIT_DEBUG || true
export GIT_TERMINAL_PROMPT=0

REF="${1:?usage: github-mirror.sh <full ref>}"
case "$REF" in refs/heads/*|refs/tags/*) ;; *) echo "unsupported ref: $REF" >&2; exit 2;; esac
: "${GH_MIRROR_ACTOR:?GH_MIRROR_ACTOR is not set}"
: "${GH_MIRROR_REPO:?GH_MIRROR_REPO is not set}"
: "${GH_MIRROR_TOKEN:?GH_MIRROR_TOKEN is not set}"

ASKPASS="$(mktemp)"
trap 'rm -f "$ASKPASS"' EXIT
cat >"$ASKPASS" <<'SH'
#!/bin/sh
case "$1" in
  *sername*) printf '%s\n' "$GH_MIRROR_ACTOR" ;;
  *) printf '%s\n' "$GH_MIRROR_TOKEN" ;;
esac
SH
chmod 700 "$ASKPASS"
export GIT_ASKPASS="$ASKPASS"

URL="https://github.com/${GH_MIRROR_ACTOR}/${GH_MIRROR_REPO}.git"
git push "$URL" "${REF}:${REF}" 2>&1 \
  | sed -e "s#${GH_MIRROR_TOKEN}#***#g" -e 's#https://[^/ @]*@#https://***@#g'
