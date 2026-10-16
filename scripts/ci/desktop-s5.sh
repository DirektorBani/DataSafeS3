#!/bin/sh
# S5 for the Desktop 1.0 shell: CSP default-src 'self', connect-src is only
# Tauri IPC, capabilities name permissions and do not grant shell:default,
# and the shell does not put a secret on a process argument.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"
app=clients/desktop

python3 - "$app/src-tauri/tauri.conf.json" "$app/src-tauri/capabilities/default.json" << 'PY'
import json, pathlib, sys
conf = json.loads(pathlib.Path(sys.argv[1]).read_text())
csp = conf["app"]["security"]["csp"]
if not isinstance(csp, str) or not csp.strip():
    raise SystemExit("CSP is empty")
parts = {}
for item in csp.split(";"):
    item = item.strip()
    if not item:
        continue
    key, _, value = item.partition(" ")
    parts[key] = value.strip()
if parts.get("default-src") != "'self'":
    raise SystemExit(f"default-src must be 'self', got {parts.get('default-src')!r}")
connect = parts.get("connect-src", "").split()
allowed = {"ipc:", "http://ipc.localhost"}
extra = [token for token in connect if token not in allowed]
if not connect or extra:
    raise SystemExit(f"connect-src must be only Tauri IPC, got {connect!r}")
caps = json.loads(pathlib.Path(sys.argv[2]).read_text())
permissions = caps.get("permissions") or []
if not isinstance(permissions, list) or not permissions:
    raise SystemExit("capabilities must name permissions")
if not all(isinstance(item, str) for item in permissions):
    raise SystemExit("permissions must be names")
granted = [item for item in permissions if item == "shell:default" or item.startswith("shell:")]
if granted:
    raise SystemExit(f"capabilities grant shell: {granted}")
print("S5 config: ok")
PY

if grep -R -n --include='*.rs' --include='*.ts' --include='*.tsx' --include='*.js' --include='*.json' -e '--password' -e 'shell:default' "$app" >/dev/null; then
  echo "S5: secret argument or shell:default found under $app" >&2
  grep -R -n --include='*.rs' --include='*.ts' --include='*.tsx' --include='*.js' --include='*.json' -e '--password' -e 'shell:default' "$app" >&2 || true
  exit 1
fi
echo "S5 sources: ok"
