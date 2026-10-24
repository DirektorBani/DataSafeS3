#!/usr/bin/env bash
# Push GitHub branches and tags to the GitVerse repository.
#
# HTTPS (default): username DirektorBani, token as the password.
#   https://gitverse.ru/docs/collaborative/repositories/guides/repository-workflow/clone-repository
#   https://gitverse.ru/docs/collaborative/authentification/work-over-https
#   https://gitverse.ru/docs/collaborative/authentification/tokens
# SSH fallback: private key from GITVERSE_SSH_KEY, host key pinned to the
# published GitVerse fingerprint.
#   https://gitverse.ru/docs/collaborative/authentification/ssh-keys
#   https://gitverse.ru/docs/collaborative/authentification/gitverse-ssh-key-fingerprints
#
# This is not git push --mirror. Refs that exist only on GitVerse are left in
# place, including branches or tags removed on GitHub. A non-fast-forward
# update is rejected unless force is enabled.
#
# Do not enable xtrace: the token and the SSH key must not appear in the log.
# The remote URL never contains the token. GIT_ASKPASS answers the password
# prompt. stdout and stderr of git push are redacted before they are printed.
set -euo pipefail

unset GIT_TRACE GIT_TRACE_PACKET GIT_CURL_VERBOSE GIT_DEBUG GIT_TRACE_PERFORMANCE || true
export GIT_TERMINAL_PROMPT=0

GITVERSE_USER="${GITVERSE_USER:-DirektorBani}"
GITVERSE_REPO="${GITVERSE_REPO:-DataSafeS3}"
GITVERSE_HOST="${GITVERSE_HOST:-gitverse.ru}"
SOURCE_REMOTE="${SOURCE_REMOTE:-origin}"
WORKDIR="${GITVERSE_MIRROR_WORKDIR:-$PWD}"

# Published RSA host key. Fingerprint SHA256:IL4khwaSrT4uHCu9LTUW7dRJyMB5XkOJnFrP0nmdMXU
# https://gitverse.ru/docs/collaborative/authentification/gitverse-ssh-key-fingerprints
GITVERSE_HOSTKEY='gitverse.ru ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCr4Mgh2eX+tkxoVcO4BQDDkC8HqJsUiKivWc0R6umR7eQM6kBsq/LgGgO7Xqts1oHl0/QhF307t7Icwf6S3pGRw5SkQC+VdIEzre/rDlDeVbU+Mzwc5GPCJB96OlOZMUVJ/xhHkw3vUowMMiShOZxpdBjKGpb/oD2hz5UY2aheDivjq1lCennAv0iN6CJfJFRHHWb6vgRMznxC8qoce3fVzcYzFd2gXA2iupdqI+6dUZsIuhDS76W3Yc/UrHOQey3NJZonsqXepfIh7GXjt4X0DQcw8fA2SMFoxmDyHMFVeTHLj1P/S1V+8NtOlqwT+ntCN4rNYtqXXbXNUw6OhThvuusmw21kgaCm9CjSSG54wf3lp+7bEn3Tkfu8Byb8hYRtqY4U+IRrSaDfdyUO8GKCNRFEnoC3c3AeTjPQF+jyx5bYbPZLPnVrPYWBNMwgA79OS/3BTOM4EuAOzLLTITew5nxjPnuwzMChr+j6+8aGaQAjNGWv7KbZxbCFxYkrwKo7Bb9wQsvGEUpzi5AGG00SrUkq1JjCf3Ep7YNqRA7HK6i8QukBOkwJ/LgI1csIG94AcWAhICoeal7XBRjZl/EiDrgbDJp/5TKtrvKzuMeRUiH6ATz6QDi2AcCFjcAv7I0HWlphwJvlN/Um0vg71h6zK/k053VhJucMxXODC6SCrw=='

ASKPASS=""
KEYFILE=""
KNOWN=""

cleanup() {
  rm -f "$ASKPASS" "$KEYFILE" "$KNOWN"
}
trap cleanup EXIT

redact() {
  python3 -c '
import os, re, sys
data = sys.stdin.read()
token = os.environ.get("GITVERSE_TOKEN", "")
key = os.environ.get("GITVERSE_SSH_KEY", "")
if token:
    data = data.replace(token, "***")
if key.strip():
    data = data.replace(key, "***")
data = re.sub(r"https://[^/\s@]+@", "https://***@", data)
sys.stdout.write(data)
'
}

notice() {
  printf '%s\n' "$1"
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    printf '%s\n' "$1" >> "$GITHUB_STEP_SUMMARY"
  fi
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    printf '::notice title=GitVerse mirror::%s\n' "$1"
  fi
}

lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

parse_bool() {
  case "$(lower "$1")" in
    true|1|yes) printf '1' ;;
    false|0|no) printf '0' ;;
    *) return 1 ;;
  esac
}

cd "$WORKDIR"
git rev-parse --is-inside-work-tree >/dev/null

EVENT_NAME="${EVENT_NAME:-}"
if [ "$EVENT_NAME" = "workflow_dispatch" ]; then
  FORCE_RAW="${INPUT_FORCE:-false}"
else
  FORCE_RAW="${INPUT_FORCE:-}"
  if [ -z "$FORCE_RAW" ]; then
    FORCE_RAW="${VAR_FORCE:-${GITVERSE_MIRROR_FORCE:-false}}"
  fi
fi
if ! FORCE="$(parse_bool "$FORCE_RAW")"; then
  echo "GITVERSE_MIRROR_FORCE must be true or false, got: ${FORCE_RAW}" >&2
  exit 1
fi

AUTH_RAW="${GITVERSE_MIRROR_AUTH:-https}"
AUTH="$(lower "$AUTH_RAW")"
case "$AUTH" in
  https|ssh) ;;
  *)
    echo "GITVERSE_MIRROR_AUTH must be https or ssh, got: ${AUTH_RAW}" >&2
    exit 1
    ;;
esac

HAVE_TOKEN=0
HAVE_SSH=0
if [ -n "${GITVERSE_TOKEN:-}" ]; then
  HAVE_TOKEN=1
fi
if [ -n "${GITVERSE_SSH_KEY:-}" ]; then
  HAVE_SSH=1
fi

if [ "$HAVE_TOKEN" -eq 0 ] && [ "$HAVE_SSH" -eq 0 ]; then
  notice "GITVERSE_TOKEN and GITVERSE_SSH_KEY are not set. Mirror skipped. Nothing was pushed."
  exit 0
fi

if [ "$AUTH" = "https" ] && [ "$HAVE_TOKEN" -eq 0 ]; then
  notice "GITVERSE_MIRROR_AUTH=https but GITVERSE_TOKEN is empty. Using the SSH key."
  AUTH=ssh
fi
if [ "$AUTH" = "ssh" ] && [ "$HAVE_SSH" -eq 0 ]; then
  notice "GITVERSE_MIRROR_AUTH=ssh but GITVERSE_SSH_KEY is empty. Using HTTPS and GITVERSE_TOKEN."
  AUTH=https
fi

DRY_RUN=0
if [ "${GITVERSE_MIRROR_DRY_RUN:-0}" = "1" ]; then
  DRY_RUN=1
fi

if [ -n "${GITVERSE_MIRROR_URL:-}" ]; then
  REMOTE_URL="$GITVERSE_MIRROR_URL"
elif [ "$AUTH" = "ssh" ]; then
  REMOTE_URL="git@${GITVERSE_HOST}:${GITVERSE_USER}/${GITVERSE_REPO}.git"
else
  REMOTE_URL="https://${GITVERSE_HOST}/${GITVERSE_USER}/${GITVERSE_REPO}.git"
fi

if [ -f "$(git rev-parse --git-path shallow)" ]; then
  git fetch --unshallow "$SOURCE_REMOTE"
fi

git fetch --prune --no-tags "$SOURCE_REMOTE" \
  "+refs/heads/*:refs/mirror/heads/*" \
  "+refs/tags/*:refs/mirror/tags/*"

if [ "$AUTH" = "https" ]; then
  ASKPASS=$(mktemp)
  cat > "$ASKPASS" << 'EOF'
#!/bin/sh
case "$1" in
  *[Uu]sername*) printf '%s\n' "${GITVERSE_USER}" ;;
  *) printf '%s\n' "${GITVERSE_TOKEN}" ;;
esac
EOF
  chmod 700 "$ASKPASS"
  export GIT_ASKPASS="$ASKPASS"
  if [ -n "${GITVERSE_MIRROR_ASKPASS_COPY:-}" ]; then
    cp "$ASKPASS" "$GITVERSE_MIRROR_ASKPASS_COPY"
    chmod 700 "$GITVERSE_MIRROR_ASKPASS_COPY"
  fi
else
  case "$GITVERSE_SSH_KEY" in
    *"PRIVATE KEY"*) ;;
    *)
      echo "GITVERSE_SSH_KEY does not look like a PEM private key. The key text was not printed." >&2
      exit 1
      ;;
  esac
  KEYFILE=$(mktemp)
  KNOWN=$(mktemp)
  printf '%s\n' "$GITVERSE_SSH_KEY" > "$KEYFILE"
  printf '\n' >> "$KEYFILE"
  chmod 600 "$KEYFILE"
  printf '%s\n' "$GITVERSE_HOSTKEY" > "$KNOWN"
  chmod 600 "$KNOWN"
  export GIT_SSH_COMMAND="ssh -i ${KEYFILE} -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=yes -o HashKnownHosts=no -o UserKnownHostsFile=${KNOWN} -o CheckHostIP=no"
fi

push_one() {
  local src="$1" dst="$2"
  local -a cmd
  cmd=(git -c credential.helper= push --porcelain)
  if [ "$FORCE" -eq 1 ]; then
    cmd+=(--force)
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    printf 'would push %s -> %s\n' "$src" "$dst"
    return 0
  fi
  local out
  if out=$("${cmd[@]}" "$REMOTE_URL" "${src}:${dst}" 2>&1); then
    printf '%s\n' "$out" | redact
    return 0
  else
    printf '%s\n' "$out" | redact
    return 1
  fi
}

FAILED=0
HINT=0
BRANCH_OK=0
BRANCH_BAD=0
TAG_OK=0
TAG_BAD=0

reject_hint() {
  if [ "$HINT" -eq 1 ]; then
    return 0
  fi
  HINT=1
  echo "A branch or tag on GitVerse is not a fast-forward of GitHub. Re-run the Mirror GitVerse workflow with force, or set the repository variable GITVERSE_MIRROR_FORCE=true. Force overwrites diverged branches and moved tags. It does not delete refs that exist only on GitVerse." >&2
}

push_group() {
  local kind="$1" prefix="$2" dest_prefix="$3"
  local name src dst full
  local -a names=()
  mapfile -t names < <(git for-each-ref --format='%(refname:strip=3)' "$prefix" | sort)
  local -a ordered=()
  local n
  for n in "${names[@]+"${names[@]}"}"; do
    if [ "$n" = "main" ]; then
      ordered+=("$n")
    fi
  done
  for n in "${names[@]+"${names[@]}"}"; do
    if [ "$n" != "main" ]; then
      ordered+=("$n")
    fi
  done
  for name in "${ordered[@]+"${ordered[@]}"}"; do
    if [ "$kind" = "tag" ]; then
      full="refs/tags/${name}"
    else
      full="refs/heads/${name}"
    fi
    if ! git check-ref-format "$full"; then
      echo "refusing unsafe ${kind} name" >&2
      FAILED=1
      if [ "$kind" = "branch" ]; then
        BRANCH_BAD=$((BRANCH_BAD + 1))
      else
        TAG_BAD=$((TAG_BAD + 1))
      fi
      continue
    fi
    src="${prefix}/${name}"
    dst="${dest_prefix}/${name}"
    if push_one "$src" "$dst"; then
      if [ "$kind" = "branch" ]; then
        BRANCH_OK=$((BRANCH_OK + 1))
      else
        TAG_OK=$((TAG_OK + 1))
      fi
    else
      FAILED=1
      reject_hint
      if [ "$kind" = "branch" ]; then
        BRANCH_BAD=$((BRANCH_BAD + 1))
      else
        TAG_BAD=$((TAG_BAD + 1))
      fi
    fi
  done
}

if [ -z "$(git for-each-ref --format='%(refname)' refs/mirror/heads)" ]; then
  echo "No branches were fetched from ${SOURCE_REMOTE}. Nothing was pushed." >&2
  exit 1
fi

push_group branch refs/mirror/heads refs/heads
push_group tag refs/mirror/tags refs/tags

SUMMARY="GitVerse mirror (${AUTH}, force=$( [ "$FORCE" -eq 1 ] && printf true || printf false ), dry_run=${DRY_RUN}): branches ok=${BRANCH_OK} failed=${BRANCH_BAD}; tags ok=${TAG_OK} failed=${TAG_BAD}. Refs that exist only on GitVerse were not deleted."
notice "$SUMMARY"

if [ "$FAILED" -ne 0 ]; then
  exit 1
fi
