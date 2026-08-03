#!/usr/bin/env bash
# Push rendered bundle + remote scripts to each node and run apply-node.sh.
# SECURITY: BatchMode + StrictHostKeyChecking=yes (accept-new only with --lab).
# Compatible with bash 3.2+.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

BUNDLE=""
INVENTORY=""
IDENTITY=""
SSH_USER=""
SSH_MODE="K"
LEADER_IP=""
VIP_MODE="subnet"
INTERFACE="eth0"
DRY_RUN=0
SKIP_HEALTH=0
LAB=0
DEPLOY_STORAGE=1
NODES_FILE=""

ok()   { printf '  [OK] %s\n' "$*" >&2; }
warn() { printf '  [!!] %s\n' "$*" >&2; }
err()  { printf '  [XX] %s\n' "$*" >&2; exit 1; }
info() { printf '  %s\n' "$*" >&2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --bundle) BUNDLE="$2"; shift 2 ;;
    --inventory) INVENTORY="$2"; shift 2 ;;
    --identity) IDENTITY="$2"; shift 2 ;;
    --ssh-user) SSH_USER="$2"; shift 2 ;;
    --ssh-mode) SSH_MODE="$2"; shift 2 ;;
    --leader-ip) LEADER_IP="$2"; shift 2 ;;
    --vip-mode) VIP_MODE="$2"; shift 2 ;;
    --interface) INTERFACE="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --skip-health) SKIP_HEALTH=1; shift ;;
    --lab) LAB=1; shift ;;
    --no-storage) DEPLOY_STORAGE=0; shift ;;
    --nodes-file) NODES_FILE="$2"; shift 2 ;;
    *) err "unknown: $1" ;;
  esac
done

[ -n "$BUNDLE" ] && [ -d "$BUNDLE" ] || err "--bundle dir required"
[ -f "$BUNDLE/plan.json" ] || err "bundle missing plan.json"
[ -d "$ROOT/scripts/cluster/remote" ] || err "remote scripts missing"

if [ -z "$LEADER_IP" ]; then
  LEADER_IP="$(sed -n 's/.*"leader_ip"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$BUNDLE/plan.json" | head -n1)"
fi
[ -n "$LEADER_IP" ] || err "leader_ip unknown"

if [ -z "$SSH_USER" ] && [ -n "$INVENTORY" ] && [ -f "$INVENTORY" ]; then
  SSH_USER="$(sed -n 's/.*"ssh_user"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$INVENTORY" | head -n1)"
  SSH_MODE="$(sed -n 's/.*"ssh_mode"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$INVENTORY" | head -n1)"
fi
[ -n "$SSH_USER" ] || SSH_USER="datasafes3"
[ -n "$SSH_MODE" ] || SSH_MODE="K"

if [ "$SSH_MODE" = "P" ] && [ -z "$IDENTITY" ]; then
  err "Apply with ssh_mode=P requires --identity after datasafes3 keys exist (password never on argv)"
fi
if [ "$SSH_MODE" = "K" ] && [ -z "$IDENTITY" ]; then
  warn "no --identity: relying on ssh-agent / default keys"
fi

# rows: fabric_ip ssh_host ssh_port
NODES_TMP="$(mktemp)"
trap 'rm -f "$NODES_TMP"' EXIT

if [ -n "$NODES_FILE" ] && [ -f "$NODES_FILE" ]; then
  grep -v '^\s*#' "$NODES_FILE" | grep -v '^\s*$' >"$NODES_TMP" || true
elif [ -n "$INVENTORY" ] && [ -f "$INVENTORY" ]; then
  companion="$(dirname "$INVENTORY")/lab-nodes.txt"
  # Only use companion map for lab inventories / --lab (avoid polluting normal DryRun)
  case "$(basename "$INVENTORY")" in
    inventory-lab.json) use_companion=1 ;;
    *) use_companion=0 ;;
  esac
  [ "$LAB" = "1" ] && use_companion=1
  if [ "$use_companion" = "1" ] && [ -f "$companion" ]; then
    grep -v '^\s*#' "$companion" | grep -v '^\s*$' >"$NODES_TMP" || true
  fi
fi

if [ ! -s "$NODES_TMP" ]; then
  for d in "$BUNDLE"/nodes/*; do
    [ -d "$d" ] || continue
    ip="$(basename "$d")"
    echo "$ip $ip 22" >>"$NODES_TMP"
  done
fi

count="$(wc -l <"$NODES_TMP" | tr -d ' ')"
[ "$count" -ge 3 ] || err "need >=3 nodes"

STATE_DIR="${HOME:-/tmp}/.datasafe-cluster"
mkdir -p "$STATE_DIR"
KNOWN="$STATE_DIR/known_hosts"
[ "$LAB" = "1" ] && KNOWN="$STATE_DIR/lab_known_hosts"

ssh_opts=(-o ConnectTimeout=15 -o UserKnownHostsFile="$KNOWN")
if [ "$LAB" = "1" ]; then
  ssh_opts+=(-o StrictHostKeyChecking=accept-new)
else
  ssh_opts+=(-o StrictHostKeyChecking=yes)
fi
if [ "$SSH_MODE" = "K" ] || [ -n "$IDENTITY" ]; then
  ssh_opts+=(-o BatchMode=yes)
fi
[ -n "$IDENTITY" ] && ssh_opts+=(-i "$IDENTITY")

REMOTE_ROOT="/var/tmp/datasafe-apply"
fail=0

while read -r fabric_ip ssh_host ssh_port; do
  [ -n "$fabric_ip" ] || continue
  info "--- node fabric=$fabric_ip ssh=${ssh_host}:${ssh_port} ---"
  remote_cmd="rm -rf ${REMOTE_ROOT} && mkdir -p -m 700 ${REMOTE_ROOT} && echo ready"
  apply_cmd="sudo bash ${REMOTE_ROOT}/remote/apply-node.sh --bundle ${REMOTE_ROOT}/bundle --node-ip ${fabric_ip} --leader-ip ${LEADER_IP} --vip-mode ${VIP_MODE} --interface ${INTERFACE}"
  # Deploy storage AFTER all members are up (lab etcd/Patroni quorum); skip during per-node apply when --lab.
  if [ "$DEPLOY_STORAGE" = "1" ] && [ "$LAB" != "1" ]; then
    apply_cmd="${apply_cmd} --deploy-storage"
  fi

  if [ "$DRY_RUN" = "1" ]; then
    info "DRY-RUN ssh -p ${ssh_port} ${SSH_USER}@${ssh_host} '$remote_cmd'"
    info "DRY-RUN scp -P ${ssh_port} -r bundle+remote -> ${SSH_USER}@${ssh_host}:${REMOTE_ROOT}/"
    info "DRY-RUN ssh -p ${ssh_port} ${SSH_USER}@${ssh_host} '$apply_cmd'"
    continue
  fi

  command -v ssh >/dev/null 2>&1 || err "ssh client missing"
  command -v scp >/dev/null 2>&1 || err "scp client missing"

  # ssh must not steal the nodes-file stdin (otherwise only the first node is applied).
  if ! ssh -n "${ssh_opts[@]}" -p "$ssh_port" "${SSH_USER}@${ssh_host}" "$remote_cmd"; then
    warn "ssh prep failed for $fabric_ip"
    fail=$((fail + 1))
    continue
  fi
  if ! scp "${ssh_opts[@]}" -P "$ssh_port" -r \
    "$BUNDLE" "${SSH_USER}@${ssh_host}:${REMOTE_ROOT}/bundle"; then
    warn "scp bundle failed for $fabric_ip"
    fail=$((fail + 1))
    continue
  fi
  if ! scp "${ssh_opts[@]}" -P "$ssh_port" -r \
    "$ROOT/scripts/cluster/remote" "${SSH_USER}@${ssh_host}:${REMOTE_ROOT}/remote"; then
    warn "scp remote scripts failed for $fabric_ip"
    fail=$((fail + 1))
    continue
  fi
  if ! ssh -n "${ssh_opts[@]}" -p "$ssh_port" "${SSH_USER}@${ssh_host}" "$apply_cmd"; then
    warn "apply-node failed for $fabric_ip"
    fail=$((fail + 1))
    continue
  fi
  ok "applied $fabric_ip"
done <"$NODES_TMP"

if [ "$DRY_RUN" = "1" ]; then
  ok "push Apply DryRun planned for $count nodes"
  exit 0
fi

if [ "$fail" -gt 0 ]; then
  err "$fail node(s) failed Apply"
fi

# Lab: wait for Patroni primary, then deploy storage on leader (shared net namespace).
if [ "$LAB" = "1" ] && [ "$DEPLOY_STORAGE" = "1" ] && [ "$DRY_RUN" = "0" ]; then
  leader_ssh_host=""
  leader_ssh_port=""
  while read -r fabric_ip ssh_host ssh_port; do
    [ "$fabric_ip" = "$LEADER_IP" ] || continue
    leader_ssh_host="$ssh_host"
    leader_ssh_port="$ssh_port"
  done <"$NODES_TMP"
  [ -n "$leader_ssh_host" ] || err "leader SSH mapping missing for storage deploy"

  info "waiting for Patroni primary on leader before storage deploy..."
  ready=0
  for _ in $(seq 1 90); do
    if ssh -n "${ssh_opts[@]}" -p "$leader_ssh_port" "${SSH_USER}@${leader_ssh_host}" \
      "curl -sf http://127.0.0.1:8008/primary >/dev/null"; then
      ready=1
      break
    fi
    sleep 2
  done
  if [ "$ready" != "1" ]; then
    warn "Patroni primary not ready in time — attempting storage deploy anyway"
  else
    ok "Patroni primary ready"
  fi

  storage_cmd="sudo bash ${REMOTE_ROOT}/remote/deploy-storage-server.sh --leader-ip ${LEADER_IP} --bundle ${REMOTE_ROOT}/bundle"
  if ssh -n "${ssh_opts[@]}" -p "$leader_ssh_port" "${SSH_USER}@${leader_ssh_host}" "$storage_cmd"; then
    ok "storage-server deployed on leader after quorum"
  else
    warn "storage-server deploy failed on leader"
    fail=$((fail + 1))
  fi
  [ "$fail" -eq 0 ] || err "$fail failure(s) after storage deploy"
fi

if [ "$SKIP_HEALTH" = "0" ]; then
  # From host: health against leader SSH tunnel ports if lab, else fabric IP
  HEALTH_HOST="$LEADER_IP"
  if [ "$LAB" = "1" ]; then
    HEALTH_HOST="127.0.0.1"
    # storage published on 9010 in lab compose
    if curl -fsS --max-time 5 "http://127.0.0.1:9010/healthz" >/dev/null 2>&1; then
      ok "lab storage healthz :9010"
    else
      warn "lab storage healthz not ready on :9010"
    fi
  fi
  bash "$ROOT/scripts/cluster/remote/health-gates.sh" \
    --leader-ip "$HEALTH_HOST" \
    || warn "health gates not green yet"
fi

ok "cluster push Apply finished"
