#!/bin/sh
# Installed as /usr/local/bin/datasafe-entrypoint.
# Windows checkouts can deliver CRLF. A CR on the shebang is "no such file or
# directory", so the launcher strips CR from /docker-entrypoint.sh at start. This
# also covers a bind-mount over /docker-entrypoint.sh (HA local and local-binary
# overlays).
set -e
tr -d "\015" < /docker-entrypoint.sh > /tmp/docker-entrypoint.lf
chmod +x /tmp/docker-entrypoint.lf
exec /tmp/docker-entrypoint.lf "$@"
