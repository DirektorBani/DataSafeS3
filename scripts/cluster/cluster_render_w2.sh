#!/usr/bin/env bash
# Wave 2 config render (bash) — parity with ClusterRender.ps1 DryRun.
# Compatible with bash 3.2+. Secrets redacted by default.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TMPL="$ROOT/deploy/cluster/templates"

INVENTORY=""
OUT_DIR=""
VIP_S3="10.0.0.10"
VIP_CONSOLE="10.0.0.11"
VIP_PG="10.0.0.12"
LEADER_IP=""
INTERFACE="eth0"
REDACT=1
LAYOUT=""
VIP_MODE=""

ok()   { printf '  [OK] %s\n' "$*" >&2; }
err()  { printf '  [XX] %s\n' "$*" >&2; exit 1; }
warn() { printf '  [!!] %s\n' "$*" >&2; }

usage() {
  cat <<'EOF'
Usage: cluster_render_w2.sh --inventory FILE --out-dir DIR [options]
  --vip-s3 IP --vip-console IP --vip-postgres IP
  --leader IP --interface IFACE
  --no-redact   (write real secrets; only for live Apply host state)
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --inventory) INVENTORY="$2"; shift 2 ;;
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    --vip-s3) VIP_S3="$2"; shift 2 ;;
    --vip-console) VIP_CONSOLE="$2"; shift 2 ;;
    --vip-postgres) VIP_PG="$2"; shift 2 ;;
    --leader) LEADER_IP="$2"; shift 2 ;;
    --interface) INTERFACE="$2"; shift 2 ;;
    --no-redact) REDACT=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) err "unknown arg: $1" ;;
  esac
done

[ -n "$INVENTORY" ] && [ -f "$INVENTORY" ] || err "--inventory required"
[ -n "$OUT_DIR" ] || err "--out-dir required"
[ -d "$TMPL" ] || err "templates missing: $TMPL"

# Extract simple JSON fields without jq (bash 3.2)
json_field() {
  # $1=file $2=key -> first string value
  sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" "$1" | head -n1
}

LAYOUT="$(json_field "$INVENTORY" layout)"
[ -n "$LAYOUT" ] || LAYOUT="production"
VIP_MODE="$(json_field "$INVENTORY" vip_mode)"
[ -n "$VIP_MODE" ] || VIP_MODE="subnet"

# Collect node IPs (order preserved) — one match per occurrence
IPS=()
while IFS= read -r line; do
  [ -n "$line" ] && IPS+=("$line")
done <<EOF
$(grep -oE '"ip"[[:space:]]*:[[:space:]]*"[^"]+"' "$INVENTORY" | sed 's/.*"\([^"]*\)"$/\1/')
EOF

[ "${#IPS[@]}" -ge 3 ] || err "need >=3 node ips in inventory"
if [ -z "$LEADER_IP" ]; then
  LEADER_IP="${IPS[0]}"
fi

NEED=6
[ "$LAYOUT" = "dev" ] && NEED=3

if [ "$REDACT" = "1" ]; then
  PG_SUPER="REDACTED_PG_SUPER"
  PG_REPL="REDACTED_PG_REPL"
  AUTH_PASS="REDACTED"
else
  # openssl preferred; fallback to /dev/urandom
  rand() {
    if command -v openssl >/dev/null 2>&1; then
      openssl rand -hex 16
    else
      od -An -N16 -tx1 /dev/urandom | tr -d ' \n'
    fi
  }
  PG_SUPER="$(rand)"
  PG_REPL="$(rand)"
  AUTH_PASS="$(rand | cut -c1-8)"
fi

# /24 from leader for pg_hba
IFS=. read -r a b c d <<EOF
$LEADER_IP
EOF
PG_HBA_CIDR="${a}.${b}.${c}.0/24"

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/lb" "$OUT_DIR/nfs" "$OUT_DIR/bootstrap" "$OUT_DIR/nodes"

# etcd initial cluster string
ETCD_INITIAL=""
ETCD_HOSTS=""
i=0
for ip in "${IPS[@]}"; do
  [ -n "$ETCD_INITIAL" ] && ETCD_INITIAL="${ETCD_INITIAL},"
  ETCD_INITIAL="${ETCD_INITIAL}etcd${i}=http://${ip}:2380"
  [ -n "$ETCD_HOSTS" ] && ETCD_HOSTS="${ETCD_HOSTS},"
  ETCD_HOSTS="${ETCD_HOSTS}${ip}:2379"
  i=$((i + 1))
done

expand_file() {
  # stdin template -> stdout with {{KEY}} replaced from env-like vars passed as KEY=val args after --
  local content key val
  # Normalize CRLF templates (Windows checkout) to LF so etcd/env sourcing works on Linux.
  content="$(tr -d '\r')"
  while [ $# -gt 0 ]; do
    key="$1"
    val="$2"
    shift 2
    # escape & \ for sed replacement carefully — use bash replace
    content="${content//\{\{$key\}\}/$val}"
  done
  # fail if placeholders remain (except we allow none)
  if printf '%s' "$content" | grep -qE '\{\{[A-Z0-9_]+\}\}'; then
    err "unresolved placeholders in template"
  fi
  printf '%s\n' "$content"
}

# Per-node etcd + patroni
i=0
for ip in "${IPS[@]}"; do
  ndir="$OUT_DIR/nodes/$ip"
  mkdir -p "$ndir"
  expand_file \
    ETCD_NAME "etcd$i" \
    NODE_IP "$ip" \
    ETCD_INITIAL_CLUSTER "$ETCD_INITIAL" \
    <"$TMPL/etcd/etcd.env.tmpl" >"$ndir/etcd.env"

  expand_file \
    PATRONI_NAME "pg$i" \
    NODE_IP "$ip" \
    ETCD_HOSTS "$ETCD_HOSTS" \
    PG_REPL_PASSWORD "$PG_REPL" \
    PG_SUPER_PASSWORD "$PG_SUPER" \
    CLUSTER_CIDR "$PG_HBA_CIDR" \
    <"$TMPL/patroni/patroni.yml.tmpl" >"$ndir/patroni.yml"
  i=$((i + 1))
done

# HAProxy backends
CONSOLE_SERVERS=""
POSTGRES_SERVERS=""
for ip in "${IPS[@]}"; do
  CONSOLE_SERVERS="${CONSOLE_SERVERS}  server console_${ip} ${ip}:8080 check port 8080
"
  POSTGRES_SERVERS="${POSTGRES_SERVERS}  server pg_${ip} ${ip}:5432 check port 8008
"
done
# trim trailing newline handled by template join

expand_file \
  LEADER_IP "$LEADER_IP" \
  VIP_S3 "$VIP_S3" \
  VIP_CONSOLE "$VIP_CONSOLE" \
  VIP_PG "$VIP_PG" \
  CONSOLE_SERVERS "$CONSOLE_SERVERS" \
  POSTGRES_SERVERS "$POSTGRES_SERVERS" \
  <"$TMPL/haproxy/haproxy.cfg.tmpl" >"$OUT_DIR/lb/haproxy.cfg"

# keepalived x3
# Optional unicast (Docker lab / firewalled L2): set UNICAST_SRC_IP + peers
UNICAST_BLOCK=""
if [ "${VIP_MODE}" = "subnet" ] && [ "${KEEPALIVED_UNICAST:-1}" = "1" ]; then
  peers=""
  for ip in "${IPS[@]}"; do
    peers="${peers}    ${ip}
"
  done
  # UNICAST_SRC_IP replaced per-node at Apply time (__NODE_IP__)
  UNICAST_BLOCK="$(printf '  unicast_src_ip __NODE_IP__\n  unicast_peer {\n%s  }\n' "$peers")"
fi

render_kv() {
  local role="$1" vrid="$2" vip="$3" check="$4" pri="$5"
  [ -n "$vip" ] || vip="0.0.0.0"
  expand_file \
    VIP_ROLE "$role" \
    CHECK_SCRIPT "$check" \
    VRRP_STATE "BACKUP" \
    INTERFACE "$INTERFACE" \
    VRID "$vrid" \
    PRIORITY "$pri" \
    AUTH_PASS "$AUTH_PASS" \
    VIP_ADDRESS "$vip" \
    UNICAST_BLOCK "$UNICAST_BLOCK" \
    <"$TMPL/keepalived/keepalived.conf.tmpl" >"$OUT_DIR/lb/keepalived-${role}.conf"
}
render_kv s3 51 "$VIP_S3" "/usr/bin/curl -sf http://127.0.0.1:9000/healthz" 100
render_kv console 52 "$VIP_CONSOLE" "/usr/bin/curl -sf http://127.0.0.1:8080/healthz" 110
render_kv postgres 53 "$VIP_PG" "/usr/bin/curl -sf http://127.0.0.1:8008/primary" 120

# NFS: compact round-robin shards
CLIENTS=""
for ip in "${IPS[@]}"; do
  CLIENTS="${CLIENTS}${ip}(rw,sync,no_subtree_check,root_squash) "
done

MOUNT_LINES=""
si=0
while [ "$si" -lt "$NEED" ]; do
  idx=$((si % ${#IPS[@]}))
  nip="${IPS[$idx]}"
  local_path="/var/lib/datasafe/nfs-export/shard${si}"
  mount_path="/var/lib/datasafe/erasure/shard${si}"
  # append export line for this node
  printf '%s %s\n' "$local_path" "$CLIENTS" >>"$OUT_DIR/nfs/exports.${nip}.lines"
  MOUNT_LINES="${MOUNT_LINES}mkdir -p \"${mount_path}\"
mount -t nfs4 ${nip}:${local_path} \"${mount_path}\" || true
"
  si=$((si + 1))
done

for expf in "$OUT_DIR/nfs"/exports.*.lines; do
  [ -f "$expf" ] || continue
  nip="${expf##*/exports.}"
  nip="${nip%.lines}"
  EXPORT_LINES="$(cat "$expf")"
  expand_file EXPORT_LINES "$EXPORT_LINES" \
    <"$TMPL/nfs/exports.tmpl" >"$OUT_DIR/nfs/exports.$nip"
  rm -f "$expf"
done

expand_file MOUNT_LINES "$MOUNT_LINES" \
  <"$TMPL/nfs/mount-shards.sh.tmpl" >"$OUT_DIR/nfs/mount-shards-leader.sh"
chmod +x "$OUT_DIR/nfs/mount-shards-leader.sh" 2>/dev/null || true

# bootstrap copies (normalize CRLF from Windows git checkout)
cp -R "$TMPL/bootstrap/." "$OUT_DIR/bootstrap/"
find "$OUT_DIR/bootstrap" -type f -print0 2>/dev/null | while IFS= read -r -d '' f; do
  tr -d '\r' <"$f" >"$f.tmp" && mv "$f.tmp" "$f"
done

# plan.json (no secrets)
{
  printf '{\n'
  printf '  "version": 2,\n'
  printf '  "leader_ip": "%s",\n' "$LEADER_IP"
  printf '  "layout": "%s",\n' "$LAYOUT"
  printf '  "shard_mount": "nfs",\n'
  printf '  "vip_mode": "%s",\n' "$VIP_MODE"
  printf '  "interface": "%s",\n' "$INTERFACE"
  printf '  "vips": {"s3": "%s", "console": "%s", "postgres": "%s"},\n' "$VIP_S3" "$VIP_CONSOLE" "$VIP_PG"
  printf '  "node_ips": ['
  i=0
  for ip in "${IPS[@]}"; do
    [ "$i" -gt 0 ] && printf ', '
    printf '"%s"' "$ip"
    i=$((i + 1))
  done
  printf '],\n'
  printf '  "shard_count": %s\n' "$NEED"
  printf '}\n'
} >"$OUT_DIR/plan.json"

# Security gate (comments stripped)
security_gate() {
  local f text rel failed=0
  while IFS= read -r -d '' f; do
    rel="${f#$OUT_DIR/}"
    text="$(grep -v '^\s*#' "$f" 2>/dev/null || true)"
    case "$rel" in
      nfs/exports.*)
        if printf '%s' "$text" | grep -qE '\*|0\.0\.0\.0/0'; then
          # allow * only inside comments (already stripped); flag any remaining *
          if printf '%s' "$text" | grep -qE '(^|[[:space:]])\*|0\.0\.0\.0/0'; then
            err "NFS/export allows world: $rel"
          fi
        fi
        ;;
      bootstrap/sudoers*|*/sudoers*)
        if printf '%s' "$text" | grep -qE 'NOPASSWD:[[:space:]]*ALL\b'; then
          err "sudoers NOPASSWD:ALL forbidden: $rel"
        fi
        ;;
      */patroni.yml)
        if printf '%s' "$text" | grep -q '0\.0\.0\.0/0'; then
          err "patroni pg_hba must not use 0.0.0.0/0: $rel"
        fi
        ;;
    esac
  done < <(find "$OUT_DIR" -type f -print0)
}

security_gate
ok "Wave 2 render: $OUT_DIR"
printf '%s\n' "$OUT_DIR"
