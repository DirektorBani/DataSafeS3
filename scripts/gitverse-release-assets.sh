#!/usr/bin/env bash
# Create (or find) the GitVerse release for a tag and attach files to it.
# GitVerse may refuse some asset extensions. With PACKAGE_FALLBACK_TGZ=1 a file
# that is refused with HTTP 4xx is attached again as <name>.tgz (the file inside).
#
#   TAG=v1.7.1 GV_TOKEN=... scripts/gitverse-release-assets.sh FILE [FILE ...]
#
# Env: GV_OWNER (DirektorBani) GV_REPO (DataSafeS3) RELEASE_NOTES_FILE (optional body)
# API: https://gitverse.ru/docs/developers/public-api/releases/api-post-releases
# Do not enable xtrace: the token is sent in a header file, never on the command line.
set -euo pipefail
: "${TAG:?TAG is required}"
: "${GV_TOKEN:?GV_TOKEN is required}"
GV_OWNER="${GV_OWNER:-DirektorBani}"
GV_REPO="${GV_REPO:-DataSafeS3}"
API="https://api.gitverse.ru/repos/${GV_OWNER}/${GV_REPO}/releases"
ACCEPT='Accept: application/vnd.gitverse.object+json;version=1'

umask 077
hdr=$(mktemp); trap 'rm -f "$hdr"' EXIT
printf 'Authorization: Bearer %s\n' "$GV_TOKEN" > "$hdr"

call() { curl --silent --show-error --http1.1 --retry 4 --retry-delay 3 --retry-all-errors -H @"$hdr" -H "$ACCEPT" "$@"; }

find_release() {
  python3 - "$TAG" <<'PY'
import json, subprocess, sys, os
tag = sys.argv[1]
api = os.environ["API"]
hdr = os.environ["HDR"]
page = 1
while True:
    out = subprocess.run(["curl","--silent","--http1.1","--retry","4","--retry-all-errors","-H","@"+hdr,
        "-H","Accept: application/vnd.gitverse.object+json;version=1",
        f"{api}?page={page}&per_page=30"],capture_output=True,text=True).stdout
    try: items = json.loads(out)
    except Exception: break
    if not isinstance(items, list) or not items: break
    for r in items:
        if r.get("tag_name") == tag:
            print(r["id"]); sys.exit(0)
    page += 1
    if page > 20: break
sys.exit(1)
PY
}
export API HDR="$hdr"

RID=$(find_release || true)
if [ -z "$RID" ]; then
  VERSION="${TAG#v}"
  notes="${RELEASE_NOTES_FILE:-}"
  body="DataSafeS3 ${TAG}. Images: gitverse.ru/direktorbani/datasafe-storage-server:${TAG}, gitverse.ru/direktorbani/datasafe-console:${TAG}. Packages: https://gitverse.ru/DirektorBani/DataSafeS3/packages"
  [ -n "$notes" ] && [ -f "$notes" ] && body="$(cat "$notes")"
  payload=$(python3 -c 'import json,sys;print(json.dumps({"tag_name":sys.argv[1],"name":"DataSafeS3 "+sys.argv[1],"body":sys.argv[2],"draft":False,"prerelease":False,"is_authorized_only":False}))' "$TAG" "$body")
  resp=$(call -X POST -H 'Content-Type: application/json' -d "$payload" "$API")
  RID=$(printf '%s' "$resp" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])') || { echo "release create failed: $resp" >&2; exit 1; }
  echo "created release id $RID for $TAG"
else
  echo "release id $RID exists for $TAG"
fi

# A re-run for the same tag replaces assets with the same name (delete, then upload).
existing=$(call "$API/$RID/assets?per_page=50" | python3 -c 'import json,sys
try: print("\n".join(str(a["id"])+" "+a["name"] for a in json.load(sys.stdin)))
except Exception: pass')
for f in "$@"; do
  name=$(basename "$f")
  old_id=$(printf '%s\n' "$existing" | awk -v n="$name" '$2==n{print $1; exit}')
  if [ -n "$old_id" ]; then
    code=$(call -o /dev/null -w '%{http_code}' -X DELETE "$API/$RID/assets/$old_id") || code=000
    case "$code" in 204|200|404) echo "replacing $name (old asset $old_id removed)" ;; *) echo "delete old $name failed HTTP $code" >&2; exit 1 ;; esac
  fi
  code=$(call -o /tmp/asset-resp.$$ -w '%{http_code}' -X POST -F "attachment=@${f}" "$API/$RID/assets?name=${name}") || code=000
  if [ "$code" != "201" ] && [ "$code" != "200" ] && [ "${PACKAGE_FALLBACK_TGZ:-0}" = "1" ] && [ "${code#4}" != "$code" ]; then
    echo "attach $name refused HTTP $code, retrying as $name.tgz" >&2
    cat /tmp/asset-resp.$$ >&2 || true; echo >&2
    tgz="$(mktemp -d)/$name.tgz"; tar -czf "$tgz" -C "$(dirname "$f")" "$name"
    tname="$name.tgz"
    old_id=$(printf '%s\n' "$existing" | awk -v n="$tname" '$2==n{print $1; exit}')
    [ -n "$old_id" ] && call -o /dev/null -X DELETE "$API/$RID/assets/$old_id" || true
    code=$(call -o /tmp/asset-resp.$$ -w '%{http_code}' -X POST -F "attachment=@${tgz}" "$API/$RID/assets?name=${tname}") || code=000
    name="$tname"
  fi
  if [ "$code" = "201" ] || [ "$code" = "200" ]; then echo "attached $name"; else echo "attach $name failed HTTP $code" >&2; cat /tmp/asset-resp.$$ >&2 || true; rm -f /tmp/asset-resp.$$; exit 1; fi
  rm -f /tmp/asset-resp.$$
done
