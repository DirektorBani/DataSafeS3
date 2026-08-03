#!/usr/bin/env bash
# Apply one node from a pushed bundle. Run as root.
# Supports systemd (prod) and /etc/datasafe/lab-mode (Docker lab, nohup).
set -euo pipefail

BUNDLE=""
NODE_IP=""
LEADER_IP=""
VIP_MODE="subnet"
INTERFACE="eth0"
DEPLOY_STORAGE=0

err() { echo "[XX] $*" >&2; exit 1; }
ok()  { echo "[OK] $*"; }

while [ $# -gt 0 ]; do
  case "$1" in
    --bundle) BUNDLE="$2"; shift 2 ;;
    --node-ip) NODE_IP="$2"; shift 2 ;;
    --leader-ip) LEADER_IP="$2"; shift 2 ;;
    --vip-mode) VIP_MODE="$2"; shift 2 ;;
    --interface) INTERFACE="$2"; shift 2 ;;
    --deploy-storage) DEPLOY_STORAGE=1; shift ;;
    *) err "unknown: $1" ;;
  esac
done

[ "$(id -u)" -eq 0 ] || err "must run as root"
[ -n "$BUNDLE" ] && [ -d "$BUNDLE" ] || err "--bundle required"
[ -n "$NODE_IP" ] || err "--node-ip required"
[ -n "$LEADER_IP" ] || err "--leader-ip required"

NODE_DIR="$BUNDLE/nodes/$NODE_IP"
[ -d "$NODE_DIR" ] || err "missing node dir: $NODE_DIR"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LAB=0
[ -f /etc/datasafe/lab-mode ] && LAB=1

have_systemd() {
  command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]
}

start_unit() {
  local name="$1"
  if have_systemd; then
    systemctl enable "$name" 2>/dev/null || true
    systemctl restart "$name" || true
  fi
}

# 1) packages (skip heavy install in lab image — already baked)
if [ "$LAB" = "1" ]; then
  ok "lab-mode: skip install-packages (prebaked image)"
else
  bash "$SCRIPT_DIR/install-packages.sh"
fi

# 2) bootstrap
if [ -f "$BUNDLE/bootstrap/create-datasafes3.sh" ]; then
  tr -d '\r' <"$BUNDLE/bootstrap/create-datasafes3.sh" >/tmp/create-datasafes3.sh
  bash /tmp/create-datasafes3.sh || true
fi
if [ "$LAB" = "0" ] && [ -f "$BUNDLE/bootstrap/sudoers-datasafes3" ]; then
  install -m 440 "$BUNDLE/bootstrap/sudoers-datasafes3" /etc/sudoers.d/datasafes3
  visudo -cf /etc/sudoers.d/datasafes3 >/dev/null
fi
if [ -f "$BUNDLE/bootstrap/keepalived-caps.override.conf" ] && have_systemd; then
  mkdir -p /etc/systemd/system/keepalived.service.d
  cp "$BUNDLE/bootstrap/keepalived-caps.override.conf" \
    /etc/systemd/system/keepalived.service.d/override.conf
fi

# Write secrets env for storage deploy (from patroni.yml if present)
if [ -f "$NODE_DIR/patroni.yml" ]; then
  if grep -q 'REDACTED_' "$NODE_DIR/patroni.yml"; then
    err "patroni.yml still has REDACTED_ secrets — re-render with --no-redact for Apply"
  fi
  # Prefer explicit superuser password (not the first password: which is replication).
  PG_SUPER="$(awk '/superuser:/{f=1} f && /password:/{gsub(/.*password:[[:space:]]*/,""); gsub(/["\r]/,""); print; exit}' "$NODE_DIR/patroni.yml")"
  if [ -z "$PG_SUPER" ]; then
    PG_SUPER="$(sed -n 's/.*password:[[:space:]]*\(.*\)/\1/p' "$NODE_DIR/patroni.yml" | tail -n1 | tr -d '\"\r')"
  fi
  umask 077
  {
    echo "PG_SUPER_PASSWORD=${PG_SUPER}"
  } >/etc/datasafe/cluster-secrets.env
fi

# 3) etcd
if [ -f "$NODE_DIR/etcd.env" ]; then
  # Strip CR from Windows-rendered env files before sourcing / passing to etcd flags.
  tr -d '\r' <"$NODE_DIR/etcd.env" >/etc/datasafe/etcd.env
  # Read into locals WITHOUT exporting — etcd rejects CLI flags shadowed by ETCD_* env.
  ETCD_NAME="$(sed -n 's/^ETCD_NAME=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  ETCD_DATA_DIR="$(sed -n 's/^ETCD_DATA_DIR=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  [ -n "$ETCD_DATA_DIR" ] || ETCD_DATA_DIR="/var/lib/datasafe/etcd"
  ETCD_LISTEN_PEER_URLS="$(sed -n 's/^ETCD_LISTEN_PEER_URLS=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  ETCD_LISTEN_CLIENT_URLS="$(sed -n 's/^ETCD_LISTEN_CLIENT_URLS=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  ETCD_INITIAL_ADVERTISE_PEER_URLS="$(sed -n 's/^ETCD_INITIAL_ADVERTISE_PEER_URLS=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  ETCD_ADVERTISE_CLIENT_URLS="$(sed -n 's/^ETCD_ADVERTISE_CLIENT_URLS=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  ETCD_INITIAL_CLUSTER="$(sed -n 's/^ETCD_INITIAL_CLUSTER=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  ETCD_INITIAL_CLUSTER_STATE="$(sed -n 's/^ETCD_INITIAL_CLUSTER_STATE=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  [ -n "$ETCD_INITIAL_CLUSTER_STATE" ] || ETCD_INITIAL_CLUSTER_STATE="new"
  ETCD_INITIAL_CLUSTER_TOKEN="$(sed -n 's/^ETCD_INITIAL_CLUSTER_TOKEN=//p' /etc/datasafe/etcd.env | head -n1 | tr -d '\r')"
  [ -n "$ETCD_INITIAL_CLUSTER_TOKEN" ] || ETCD_INITIAL_CLUSTER_TOKEN="datasafe-etcd"
  mkdir -p "${ETCD_DATA_DIR}"
  if have_systemd && systemctl list-unit-files 2>/dev/null | grep -q '^etcd'; then
    mkdir -p /etc/systemd/system/etcd.service.d
    cat >/etc/systemd/system/etcd.service.d/datasafe.conf <<'EOF'
[Service]
EnvironmentFile=/etc/datasafe/etcd.env
EOF
    systemctl daemon-reload || true
    start_unit etcd
  else
    pkill -f 'etcd --name' 2>/dev/null || true
    # Clear any ETCD_* from environment so flags win cleanly.
    nohup env -u ETCD_NAME -u ETCD_DATA_DIR -u ETCD_LISTEN_PEER_URLS -u ETCD_LISTEN_CLIENT_URLS \
      -u ETCD_INITIAL_ADVERTISE_PEER_URLS -u ETCD_ADVERTISE_CLIENT_URLS \
      -u ETCD_INITIAL_CLUSTER -u ETCD_INITIAL_CLUSTER_STATE -u ETCD_INITIAL_CLUSTER_TOKEN \
      etcd \
      --name "${ETCD_NAME}" \
      --data-dir "${ETCD_DATA_DIR}" \
      --listen-peer-urls "${ETCD_LISTEN_PEER_URLS}" \
      --listen-client-urls "${ETCD_LISTEN_CLIENT_URLS}" \
      --initial-advertise-peer-urls "${ETCD_INITIAL_ADVERTISE_PEER_URLS}" \
      --advertise-client-urls "${ETCD_ADVERTISE_CLIENT_URLS}" \
      --initial-cluster "${ETCD_INITIAL_CLUSTER}" \
      --initial-cluster-state "${ETCD_INITIAL_CLUSTER_STATE}" \
      --initial-cluster-token "${ETCD_INITIAL_CLUSTER_TOKEN}" \
      >/var/lib/datasafe/etcd.log 2>&1 &
    ok "etcd started (lab/nohup)"
  fi
fi

# 4) patroni
if [ -f "$NODE_DIR/patroni.yml" ]; then
  tr -d '\r' <"$NODE_DIR/patroni.yml" >/etc/datasafe/patroni.yml
  # Alpine lab uses /usr/libexec/postgresql16; Debian/Ubuntu use /usr/lib/postgresql/16/bin
  if [ ! -d /usr/lib/postgresql/16/bin ] && [ -d /usr/libexec/postgresql16 ]; then
    sed -i 's|bin_dir: /usr/lib/postgresql/16/bin|bin_dir: /usr/libexec/postgresql16|' /etc/datasafe/patroni.yml
  elif [ ! -d /usr/lib/postgresql/16/bin ] && [ -d /usr/libexec/postgresql ]; then
    sed -i 's|bin_dir: /usr/lib/postgresql/16/bin|bin_dir: /usr/libexec/postgresql|' /etc/datasafe/patroni.yml
  fi
  mkdir -p /var/run/postgresql /var/lib/datasafe/postgresql
  chown -R postgres:postgres /var/run/postgresql /var/lib/datasafe/postgresql 2>/dev/null || true
  chown postgres:postgres /etc/datasafe/patroni.yml 2>/dev/null || true
  chmod 600 /etc/datasafe/patroni.yml
  PATRONI_BIN="$(command -v patroni || true)"
  [ -n "$PATRONI_BIN" ] || PATRONI_BIN="/usr/local/bin/patroni"
  if have_systemd; then
    cat >/etc/systemd/system/patroni.service <<EOF
[Unit]
Description=Patroni (DataSafeS3)
After=network.target
[Service]
User=postgres
Group=postgres
ExecStart=${PATRONI_BIN} /etc/datasafe/patroni.yml
Restart=on-failure
[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload || true
    start_unit patroni
  else
    pkill -f 'patroni /etc/datasafe' 2>/dev/null || true
    # postgres user may not exist in minimal — run as postgres in lab (required by PostgreSQL).
    if id postgres >/dev/null 2>&1; then
      chown -R postgres:postgres /var/lib/datasafe/postgresql /var/run/postgresql /etc/datasafe/patroni.yml 2>/dev/null || true
      # BusyBox nohup is picky; background via shell so docker/ssh sessions can exit.
      su -s /bin/sh postgres -c "patroni /etc/datasafe/patroni.yml" >>/var/lib/datasafe/patroni.log 2>&1 &
    else
      nohup "$PATRONI_BIN" /etc/datasafe/patroni.yml >/var/lib/datasafe/patroni.log 2>&1 &
    fi
    ok "patroni started (lab/nohup)"
  fi
fi

# 5) NFS exports
EXP="$BUNDLE/nfs/exports.$NODE_IP"
if [ -f "$EXP" ]; then
  touch /etc/exports
  grep -v '# datasafe-cluster' /etc/exports > /etc/exports.datasafe.tmp || true
  {
    echo "# datasafe-cluster begin"
    grep -v '^\s*#' "$EXP" || true
    echo "# datasafe-cluster end"
  } >> /etc/exports.datasafe.tmp
  mv /etc/exports.datasafe.tmp /etc/exports
  grep -v '^\s*#' "$EXP" | awk '{print $1}' | while read -r p; do
    [ -n "$p" ] || continue
    mkdir -p "$p"
    chown datasafes3:datasafes3 "$p" 2>/dev/null || true
  done
  exportfs -ra 2>/dev/null || true
  if have_systemd; then
    start_unit nfs-server || start_unit nfs-kernel-server || true
  else
    rpcbind 2>/dev/null || true
    # best-effort in lab containers (NFS often limited)
    ok "nfs exports written (lab may lack full nfsd)"
  fi
fi

# 6) leader mounts — in lab use local bind dirs if nfs fails
if [ "$NODE_IP" = "$LEADER_IP" ]; then
  if [ -f "$BUNDLE/nfs/mount-shards-leader.sh" ]; then
    bash "$BUNDLE/nfs/mount-shards-leader.sh" 2>/dev/null || {
      ok "nfs mounts failed — creating local shard dirs for lab C1 fallback"
      need=6
      if [ -f "$BUNDLE/plan.json" ]; then
        sc="$(sed -n 's/.*"shard_count"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p' "$BUNDLE/plan.json" | head -n1)"
        [ -n "$sc" ] && need="$sc"
      fi
      i=0
      while [ "$i" -lt "$need" ]; do
        mkdir -p "/var/lib/datasafe/erasure/shard$i"
        i=$((i + 1))
      done
    }
  fi
fi

# 7) HAProxy
if [ -f "$BUNDLE/lb/haproxy.cfg" ]; then
  mkdir -p /etc/haproxy /var/run/haproxy
  tr -d '\r' <"$BUNDLE/lb/haproxy.cfg" >/etc/haproxy/haproxy.cfg
  if have_systemd; then
    start_unit haproxy
  else
    pkill haproxy 2>/dev/null || true
    nohup haproxy -f /etc/haproxy/haproxy.cfg -db >/var/lib/datasafe/haproxy.log 2>&1 &
    ok "haproxy started (lab/nohup)"
  fi
fi

# 8) keepalived — subnet VIP (real VMs + Docker SSH lab with NET_ADMIN + unicast)
if [ "$VIP_MODE" = "subnet" ]; then
  for role in s3 console postgres; do
    src="$BUNDLE/lb/keepalived-${role}.conf"
    [ -f "$src" ] || continue
    grep -q 'auth_pass REDACTED' "$src" && err "keepalived still redacted"
  done
  mkdir -p /etc/keepalived
  {
    echo "# generated by DataSafeS3 apply-node"
    for role in s3 console postgres; do
      [ -f "$BUNDLE/lb/keepalived-${role}.conf" ] && tr -d '\r' <"$BUNDLE/lb/keepalived-${role}.conf"
      echo ""
    done
  } >/etc/keepalived/keepalived.conf
  sed -i "s/interface eth0/interface ${INTERFACE}/g" /etc/keepalived/keepalived.conf || true
  # Per-node unicast source (Docker bridge / lab)
  sed -i "s/__NODE_IP__/${NODE_IP}/g" /etc/keepalived/keepalived.conf || true
  # Detect primary iface if eth0 missing (Docker often eth0 though)
  if ! ip link show "$INTERFACE" >/dev/null 2>&1; then
    det="$(ip -o link show | awk -F': ' '$2!~/lo/ {print $2; exit}')"
    if [ -n "$det" ]; then
      sed -i "s/interface ${INTERFACE}/interface ${det}/g" /etc/keepalived/keepalived.conf || true
      INTERFACE="$det"
    fi
  fi
  if have_systemd; then
    systemctl daemon-reload || true
    if ! systemctl restart keepalived; then
      ok "keepalived caps failed; fallback root"
      rm -f /etc/systemd/system/keepalived.service.d/override.conf
      systemctl daemon-reload || true
      systemctl restart keepalived || true
    fi
  else
    # Docker SSH lab (no systemd): require CAP_NET_ADMIN
    pkill keepalived 2>/dev/null || true
    keepalived -f /etc/keepalived/keepalived.conf -l || err "keepalived failed to start (need NET_ADMIN/NET_RAW)"
    sleep 1
    if pgrep keepalived >/dev/null 2>&1; then
      ok "keepalived started (lab) iface=$INTERFACE"
    else
      err "keepalived not running after start"
    fi
  fi
elif [ "$LAB" = "1" ]; then
  ok "lab-mode: vip_mode=$VIP_MODE (keepalived not required)"
fi

# 9) storage-server on leader — skipped here when DEPLOY_STORAGE=0.
# Lab/push runs storage AFTER all etcd/Patroni members are up (see cluster_push_apply.sh).
if [ "$DEPLOY_STORAGE" = "1" ] && [ "$NODE_IP" = "$LEADER_IP" ]; then
  bash "$SCRIPT_DIR/deploy-storage-server.sh" --leader-ip "$LEADER_IP" --bundle "$BUNDLE" \
    || ok "storage-server deploy deferred (docker/image may be unavailable)"
fi

ok "apply-node complete for $NODE_IP (leader=$LEADER_IP lab=$LAB)"

# Idempotent stamp (safe to re-run Apply)
mkdir -p /var/lib/datasafe/apply
{
  echo "node_ip=${NODE_IP}"
  echo "leader_ip=${LEADER_IP}"
  echo "applied_at=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date)"
  echo "lab=${LAB}"
} >/var/lib/datasafe/apply/last-apply.env
chmod 600 /var/lib/datasafe/apply/last-apply.env 2>/dev/null || true
ok "idempotent stamp written (/var/lib/datasafe/apply/last-apply.env)"
