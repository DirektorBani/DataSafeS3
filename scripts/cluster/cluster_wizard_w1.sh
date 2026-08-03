#!/usr/bin/env bash
# Cluster wizard Wave 1 (bash): inventory + schema validation. No remote Apply.
# Passwords are never written to disk. DryRun skips password prompt.
# Compatible with bash 3.2+ (macOS default).
set -euo pipefail

STATE_DIR="${HOME:-/tmp}/.datasafe-cluster"
INV_PATH="$STATE_DIR/inventory-wave1.json"
DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

ok()   { printf '  [OK] %s\n' "$*" >&2; }
warn() { printf '  [!!] %s\n' "$*" >&2; }
err()  { printf '  [XX] %s\n' "$*" >&2; }

read_ssh_mode() {
  echo "" >&2
  echo "  How to connect to nodes:" >&2
  echo "    [P] Simplified start (default): root password once → create datasafes3 + SSH keys (Apply in Wave 2)" >&2
  echo "    [K] I already have SSH keys (root or datasafes3)" >&2
  while true; do
    printf '  Select [P]: ' >&2
    read -r a || true
    a="$(printf '%s' "${a:-}" | tr '[:upper:]' '[:lower:]')"
    case "${a:-p}" in
      ''|p) echo P; return ;;
      k) echo K; return ;;
      *) warn "Enter P or K" ;;
    esac
  done
}

read_layout() {
  echo "" >&2
  echo "  Object layout (erasure):" >&2
  echo "    [R] 4+2 recommended (needs ≥6 shard paths)" >&2
  echo "    [2] 2+1 small lab (≥3 shard paths)" >&2
  while true; do
    printf '  Select [R]: ' >&2
    read -r a || true
    case "${a:-R}" in
      ''|R|r) echo production; return ;;
      2) echo dev; return ;;
      *) warn "Enter R or 2" ;;
    esac
  done
}

read_vip_mode() {
  echo "" >&2
  echo "  Entry addresses:" >&2
  echo "    [V] VIP + keepalived (default; nodes + VIP same subnet)" >&2
  echo "    [D] DNS / external LB (no floating IP)" >&2
  while true; do
    printf '  Select [V]: ' >&2
    read -r a || true
    a="$(printf '%s' "${a:-}" | tr '[:upper:]' '[:lower:]')"
    case "${a:-v}" in
      ''|v) echo subnet; return ;;
      d) echo dns; return ;;
      *) warn "Enter V or D" ;;
    esac
  done
}

# Prints IPs one per line on stdout; prompts on stderr.
read_ips() {
  echo "" >&2
  echo "  Enter ≥3 node IPs (comma or space separated). Loopback not allowed." >&2
  while true; do
    printf '  Nodes: ' >&2
    read -r raw
    set --
    OLDIFS=$IFS
    IFS=' ,;'$'\t'
    # shellcheck disable=SC2086
    set -- $raw
    IFS=$OLDIFS
    count=0
    for p in "$@"; do
      [ -n "$p" ] || continue
      count=$((count + 1))
    done
    if [ "$count" -ge 3 ]; then
      for p in "$@"; do
        [ -n "$p" ] && printf '%s\n' "$p"
      done
      return
    fi
    warn "Need at least 3 IPs"
  done
}

validate_ips() {
  n=$#
  [ "$n" -ge 3 ] || { err "need at least 3 nodes"; return 1; }
  seen=" "
  for ip in "$@"; do
    case "$ip" in
      127.0.0.1|::1|localhost) err "loopback not allowed: $ip"; return 1 ;;
    esac
    case "$seen" in
      *" $ip "*) err "duplicate ip: $ip"; return 1 ;;
    esac
    seen="$seen$ip "
  done
  return 0
}

write_inventory() {
  ssh_mode="$1"
  ssh_user="$2"
  layout="$3"
  vip_mode="$4"
  shift 4
  mkdir -p "$STATE_DIR"
  umask 077
  {
    printf '{\n'
    printf '  "version": 1,\n'
    printf '  "ssh_mode": "%s",\n' "$ssh_mode"
    printf '  "ssh_user": "%s",\n' "$ssh_user"
    printf '  "layout": "%s",\n' "$layout"
    printf '  "vip_mode": "%s",\n' "$vip_mode"
    printf '  "nodes": [\n'
    i=0
    total=$#
    for ip in "$@"; do
      i=$((i + 1))
      printf '    {"ip": "%s", "ssh_port": 22, "roles": ["storage", "postgres", "etcd", "lb"]}' "$ip"
      if [ "$i" -lt "$total" ]; then printf ',\n'; else printf '\n'; fi
    done
    printf '  ]\n'
    printf '}\n'
  } >"$INV_PATH"
  if grep -qiE '"(password|root_password)"' "$INV_PATH"; then
    err "refusing to leave password fields in inventory"
    rm -f "$INV_PATH"
    return 1
  fi
  ok "Inventory written: $INV_PATH"
  ok "No password fields in inventory"
}

echo "" >&2
echo "=== Cluster mode (Wave 1) ===" >&2
echo "  Patroni / NFS / multi-LB Apply ships in Wave 2 — this wave collects inventory securely." >&2

ssh_mode="$(read_ssh_mode)"
if [ "$ssh_mode" = "P" ]; then
  ssh_user="root"
else
  printf '  SSH user [datasafes3]: ' >&2
  read -r u || true
  ssh_user="${u:-datasafes3}"
  [ -z "$ssh_user" ] && ssh_user="datasafes3"
fi
layout="$(read_layout)"
vip_mode="$(read_vip_mode)"

ips=()
while IFS= read -r line; do
  [ -n "$line" ] && ips+=("$line")
done < <(read_ips)

validate_ips "${ips[@]}"

if [ "$ssh_mode" = "P" ] && [ "$DRY_RUN" = "0" ]; then
  echo "  Root password is collected for future Wave 2 bootstrap only; it is NOT saved to disk." >&2
  printf '  Root password: ' >&2
  read -rs _cluster_pw || true
  echo "" >&2
  unset _cluster_pw
  ok "In-memory password cleared (Wave 1; Wave 2 will re-prompt at Apply)"
elif [ "$ssh_mode" = "P" ] && [ "$DRY_RUN" = "1" ]; then
  warn "DryRun: skip password prompt (would collect at Apply)"
fi

write_inventory "$ssh_mode" "$ssh_user" "$layout" "$vip_mode" "${ips[@]}"
ok "Wave 1 preflight (schema) passed"

# Wave 2 DryRun render (bash parity with PowerShell)
vip_s3="10.0.0.10"
vip_con="10.0.0.11"
vip_pg="10.0.0.12"
if [ "$vip_mode" = "subnet" ] && [ "$DRY_RUN" = "0" ]; then
  printf '  VIP-S3 [%s]: ' "$vip_s3" >&2
  read -r v || true
  [ -n "${v:-}" ] && vip_s3="$v"
  printf '  VIP-Console [%s]: ' "$vip_con" >&2
  read -r v || true
  [ -n "${v:-}" ] && vip_con="$v"
  printf '  VIP-Postgres [%s]: ' "$vip_pg" >&2
  read -r v || true
  [ -n "${v:-}" ] && vip_pg="$v"
elif [ "$vip_mode" = "subnet" ] && [ "$DRY_RUN" = "1" ]; then
  warn "DryRun: using placeholder VIPs $vip_s3 / $vip_con / $vip_pg"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
echo "" >&2
echo "  Wave 2 DryRun: rendering Patroni/etcd/HAProxy/keepalived/NFS plan..." >&2
bash "$SCRIPT_DIR/cluster_apply_w2.sh" --inventory "$INV_PATH" --dry-run \
  --vip-s3 "$vip_s3" --vip-console "$vip_con" --vip-postgres "$vip_pg" >/dev/null

echo "" >&2
echo "  Cluster Wave 1+2 DryRun complete." >&2
echo "  Live remote Apply: not in bash yet (experimental PowerShell -Apply on lab)." >&2
echo "  Security: scripts/cluster/SECURITY.md" >&2
