#!/usr/bin/env bash
# Wave 2 Apply (bash): DryRun by default; --apply pushes via SSH (mode K / --identity).
# Compatible with bash 3.2+.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
STATE_DIR="${HOME:-/tmp}/.datasafe-cluster"

INVENTORY=""
DRY_RUN=1
APPLY=0
VIP_S3="10.0.0.10"
VIP_CONSOLE="10.0.0.11"
VIP_PG="10.0.0.12"
LEADER_IP=""
IDENTITY=""
INTERFACE="eth0"
SKIP_HEALTH=0
LAB=0
NODES_FILE=""

ok()   { printf '  [OK] %s\n' "$*" >&2; }
warn() { printf '  [!!] %s\n' "$*" >&2; }
err()  { printf '  [XX] %s\n' "$*" >&2; exit 1; }
info() { printf '  %s\n' "$*" >&2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --inventory) INVENTORY="$2"; shift 2 ;;
    --vip-s3) VIP_S3="$2"; shift 2 ;;
    --vip-console) VIP_CONSOLE="$2"; shift 2 ;;
    --vip-postgres) VIP_PG="$2"; shift 2 ;;
    --leader|--leader-ip) LEADER_IP="$2"; shift 2 ;;
    --identity) IDENTITY="$2"; shift 2 ;;
    --interface) INTERFACE="$2"; shift 2 ;;
    --nodes-file) NODES_FILE="$2"; shift 2 ;;
    --vip-mode) shift 2 ;; # inventory wins; accept for CLI symmetry
    --dry-run) DRY_RUN=1; APPLY=0; shift ;;
    --apply) APPLY=1; DRY_RUN=0; shift ;;
    --skip-health) SKIP_HEALTH=1; shift ;;
    --lab) LAB=1; shift ;;
    *) err "unknown arg: $1" ;;
  esac
done

[ -n "$INVENTORY" ] || INVENTORY="$STATE_DIR/inventory-wave1.json"
[ -f "$INVENTORY" ] || err "inventory not found: $INVENTORY"

SSH_MODE="$(sed -n 's/.*"ssh_mode"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$INVENTORY" | head -n1)"
if [ "$APPLY" = "1" ] && [ "$SSH_MODE" = "P" ] && [ -z "$IDENTITY" ]; then
  err "Apply with ssh_mode=P requires --identity after datasafes3 keys exist (password never on argv)"
fi

gen_id="wave2-$(date +%Y%m%d%H%M%S 2>/dev/null || echo bash)"
OUT_DIR="$STATE_DIR/generated/$gen_id"
mkdir -p "$STATE_DIR/generated"

render_args=(--inventory "$INVENTORY" --out-dir "$OUT_DIR" \
  --vip-s3 "$VIP_S3" --vip-console "$VIP_CONSOLE" --vip-postgres "$VIP_PG" \
  --interface "$INTERFACE")
[ -n "$LEADER_IP" ] && render_args+=(--leader "$LEADER_IP")

if [ "$APPLY" = "1" ]; then
  # live Apply needs real secrets in rendered files
  render_args+=(--no-redact)
  warn "Live --apply: rendering with real secrets into $OUT_DIR (mode 700 tree)"
else
  info "DryRun: redacted render"
fi

OUT="$("$SCRIPT_DIR/cluster_render_w2.sh" "${render_args[@]}")"
chmod -R go-rwx "$OUT_DIR" 2>/dev/null || true

VIP_MODE="$(sed -n 's/.*"vip_mode"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$INVENTORY" | head -n1)"
[ -n "$VIP_MODE" ] || VIP_MODE="subnet"
LEADER_RESOLVED="$(sed -n 's/.*"leader_ip"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$OUT_DIR/plan.json" | head -n1)"

push_args=(--bundle "$OUT_DIR" --inventory "$INVENTORY" \
  --leader-ip "$LEADER_RESOLVED" --vip-mode "$VIP_MODE" --interface "$INTERFACE")
[ -n "$IDENTITY" ] && push_args+=(--identity "$IDENTITY")
[ -n "$NODES_FILE" ] && push_args+=(--nodes-file "$NODES_FILE")
[ "$SKIP_HEALTH" = "1" ] && push_args+=(--skip-health)
[ "$LAB" = "1" ] && push_args+=(--lab)

if [ "$APPLY" = "1" ]; then
  bash "$SCRIPT_DIR/cluster_push_apply.sh" "${push_args[@]}"
  ok "Wave 2 Apply finished; bundle=$OUT"
else
  bash "$SCRIPT_DIR/cluster_push_apply.sh" "${push_args[@]}" --dry-run
  info "Apply steps (planned):"
  info "  1. Bootstrap datasafes3 + scoped sudoers"
  info "  2. install-packages.sh (etcd, Patroni, Postgres, HAProxy, keepalived, NFS)"
  info "  3. deploy etcd + Patroni per node"
  info "  4. NFS export/mount C1"
  info "  5. HAProxy + keepalived"
  info "  6. health-gates.sh"
  info "Live: $0 --apply --inventory ... --identity ~/.ssh/id_ed25519"
  info "Security: scripts/cluster/SECURITY.md"
  ok "Wave 2 DryRun configs: $OUT"
fi
printf '%s\n' "$OUT"
