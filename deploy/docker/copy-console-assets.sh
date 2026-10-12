#!/bin/sh
# Copy console static files into the shared volume Caddy serves.
# CONSOLE_SRC is /srv/console in the published image, or a host dist mount.
set -eu
src="${CONSOLE_SRC:-/srv/console}"
dest=/console-assets
test -f "$src/index.html"
find "$dest" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
cp -a "$src/." "$dest/"
test -f "$dest/index.html"
grep -q 'id="root"' "$dest/index.html"
