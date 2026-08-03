#!/usr/bin/env bash
# Interactive DataSafeS3 installer (Linux / macOS / WSL / Git Bash).
# Compatible with bash 3.2+ (macOS default).
# Usage:
#   ./install.sh
#   ./install.sh --yes --profiles core,postgres,monitoring,data
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

YES=0
TAG="v1.2.0"
DATA_ROOT=""
PROFILES=""
PROJECT_NAME="datasafe"
DRY_RUN=0
CLUSTER=0

# Parallel arrays (bash 3.2-safe)
OPT_IDS=(core postgres monitoring data binary identity)
OPT_ON=(1 1 1 1 0 0)
OPT_LOCKED=(1 0 0 0 0 0)
OPT_LABEL=(
  "Core (console + S3 API)"
  "PostgreSQL metadata"
  "Monitoring (Prometheus + Grafana)"
  "Persist data on host (local-data overlay)"
  "Build from source (local Linux binary)"
  "Identity lab (LDAP + Keycloak)"
)

usage() {
  cat <<'EOF'
DataSafeS3 interactive installer

  ./install.sh
  ./install.sh --yes --profiles core,postgres,monitoring,data
  ./install.sh --dry-run --yes --profiles core,postgres,data
  ./install.sh --cluster --dry-run --yes
  ./install.sh --project datasafe --tag v1.2.0 --data-root "$HOME/.local/share/datasafe"

Profiles: core, postgres, monitoring, data, binary, identity
Cluster Wave 1+2 DryRun: inventory + render (live Apply later)
Docs: docs/getting-started/en/installer.md
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --yes|-y) YES=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --cluster) CLUSTER=1; shift ;;
    --tag) TAG="$2"; shift 2 ;;
    --data-root) DATA_ROOT="$2"; shift 2 ;;
    --profiles) PROFILES="$2"; shift 2 ;;
    --project) PROJECT_NAME="$2"; shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown arg: $1"; usage; exit 1 ;;
  esac
done

ok()   { printf '  [OK] %s\n' "$*"; }
warn() { printf '  [!!] %s\n' "$*"; }
err()  { printf '  [XX] %s\n' "$*" >&2; }
info() { printf '  %s\n' "$*"; }
title(){ printf '\n=== %s ===\n' "$*"; }

opt_index() {
  local id="$1" i=0
  for x in "${OPT_IDS[@]}"; do
    [ "$x" = "$id" ] && { echo "$i"; return 0; }
    i=$((i + 1))
  done
  return 1
}

opt_get() {
  local i
  i="$(opt_index "$1")" || return 1
  echo "${OPT_ON[$i]}"
}

opt_set() {
  local i
  i="$(opt_index "$1")" || return 1
  OPT_ON[$i]="$2"
}

set_profiles() {
  local raw="$1" p i
  [ -z "$raw" ] && return 0
  i=0
  while [ $i -lt ${#OPT_IDS[@]} ]; do
    [ "${OPT_LOCKED[$i]}" = "1" ] || OPT_ON[$i]=0
    i=$((i + 1))
  done
  OLDIFS="$IFS"
  IFS=','
  # shellcheck disable=SC2086
  set -- $raw
  IFS="$OLDIFS"
  for p in "$@"; do
    p="$(echo "$p" | tr '[:upper:]' '[:lower:]' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    [ -z "$p" ] && continue
    i="$(opt_index "$p")" || { err "Unknown profile: $p"; exit 1; }
    OPT_ON[$i]=1
  done
  opt_set core 1
}

default_data_root() {
  if [ -n "$DATA_ROOT" ]; then echo "$DATA_ROOT"; return; fi
  case "$(uname -s)" in
    Darwin) echo "$HOME/Library/Application Support/datasafe" ;;
    *) echo "$HOME/.local/share/datasafe" ;;
  esac
}

port_free() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ! ss -ltn 2>/dev/null | grep -q ":$port "
  elif command -v lsof >/dev/null 2>&1; then
    ! lsof -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
  else
    return 0
  fi
}

show_detect() {
  title "1) System"
  ok "OS: $(uname -s) $(uname -m)"
  ok "Shell: bash"
  ok "Repo: $ROOT"
}

offer_docker() {
  local os
  os="$(uname -s)"
  echo ""
  echo "  Docker is required. Choose:"
  echo "    [1] Print install docs URL"
  echo "    [2] Try package manager install"
  echo "    [3] Exit"
  printf '  Choice: '
  read -r c
  case "$c" in
    1)
      case "$os" in
        Darwin) echo "  https://docs.docker.com/desktop/setup/install/mac-install/" ;;
        Linux)  echo "  https://docs.docker.com/engine/install/" ;;
        *)      echo "  https://docs.docker.com/get-docker/" ;;
      esac
      err "Install Docker, then re-run ./install.sh"; exit 1
      ;;
    2)
      if [ "$os" = "Darwin" ] && command -v brew >/dev/null 2>&1; then
        info "brew install --cask docker"
        brew install --cask docker
        err "Start Docker Desktop, then re-run ./install.sh"; exit 1
      elif [ "$os" = "Linux" ]; then
        info "See: https://docs.docker.com/engine/install/ (needs sudo)"
        err "Install Docker Engine, then re-run ./install.sh"; exit 1
      else
        err "No supported auto path. Use option 1."; exit 1
      fi
      ;;
    *) err "Aborted: Docker required."; exit 1 ;;
  esac
}

ensure_docker() {
  title "2) Prerequisites"
  if ! command -v docker >/dev/null 2>&1; then
    err "Docker not found"
    [ "$YES" = "1" ] && { err "Docker required"; exit 1; }
    offer_docker
  fi
  if ! docker info >/dev/null 2>&1; then
    err "Docker engine not reachable"
    [ "$YES" = "1" ] && exit 1
    offer_docker
  fi
  ok "Docker Engine $(docker version --format '{{.Server.Version}}' 2>/dev/null || echo present)"
  if ! docker compose version >/dev/null 2>&1; then
    err "docker compose plugin missing"; exit 1
  fi
  ok "docker compose available"
  for p in 8080 9000 3000; do
    if port_free "$p"; then ok "Port $p free"; else warn "Port $p may be in use"; fi
  done
}

selected_profile_names() {
  local i enabled=""
  i=0
  while [ $i -lt ${#OPT_IDS[@]} ]; do
    [ "${OPT_ON[$i]}" = "1" ] && enabled="$enabled ${OPT_IDS[$i]}"
    i=$((i + 1))
  done
  # trim leading space
  printf '%s' "${enabled# }"
}

write_selection_confirmed() {
  ok "Confirmed selection: $(selected_profile_names)"
}

show_menu() {
  title "3) What to install"
  echo "  How to choose:"
  echo "    5           toggle one item"
  echo "    1,2,3,4     SET selection and continue (core always on)"
  echo "    R           recommended (1-4) and continue"
  echo "    A           all options and continue"
  echo "    C           confirm current selection and continue"
  echo "    Q           quit"
  write_opt_menu
  local i mark lock ans k part nums n count all_nums
  while true; do
    echo ""
    printf '  Select: '
    read -r ans
    ans="$(echo "$ans" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    if [ -z "$ans" ]; then write_opt_menu; continue; fi
    case "$ans" in
      Q|q) err "Aborted by user."; exit 1 ;;
      C|c)
        enforce_opt_deps
        write_selection_confirmed
        break
        ;;
      R|r)
        set_opt_selection "1 2 3 4"
        write_selection_confirmed
        break
        ;;
      A|a)
        all_nums=""
        i=1
        while [ $i -le ${#OPT_IDS[@]} ]; do
          all_nums="$all_nums $i"
          i=$((i + 1))
        done
        set_opt_selection "$all_nums"
        write_selection_confirmed
        break
        ;;
      *)
        # shellcheck disable=SC2086
        set -- $(echo "$ans" | tr ',;' '  ')
        count=$#
        if [ "$count" -eq 0 ]; then warn "Unknown input."; continue; fi
        if [ "$count" -eq 1 ]; then
          case "$1" in
            [1-9])
              if [ "$1" -ge 1 ] && [ "$1" -le ${#OPT_IDS[@]} ]; then
                i=$(($1 - 1))
                if [ "${OPT_LOCKED[$i]}" = "1" ]; then
                  warn "${OPT_LABEL[$i]} is required and stays enabled."
                else
                  if [ "${OPT_ON[$i]}" = "1" ]; then OPT_ON[$i]=0; else OPT_ON[$i]=1; fi
                  enforce_opt_deps
                fi
              else
                warn "Out of range: $1"
              fi
              write_opt_menu
              ;;
            *) warn "Unknown input. Examples: 5 | 1,2,3,4 | R | A | C" ;;
          esac
        else
          nums=""
          for part in "$@"; do
            case "$part" in
              [1-9]|[1-9][0-9]) nums="$nums $part" ;;
              *) warn "Skip: $part" ;;
            esac
          done
          # Multi-number SET = final selection → confirm and leave menu
          set_opt_selection "$nums"
          write_selection_confirmed
          break
        fi
        ;;
    esac
  done
}

enforce_opt_deps() {
  opt_set core 1
}

set_opt_selection() {
  local nums="$1" i part
  i=0
  while [ $i -lt ${#OPT_IDS[@]} ]; do
    [ "${OPT_LOCKED[$i]}" = "1" ] || OPT_ON[$i]=0
    i=$((i + 1))
  done
  # shellcheck disable=SC2086
  for part in $nums; do
    if [ "$part" -ge 1 ] && [ "$part" -le ${#OPT_IDS[@]} ]; then
      i=$((part - 1))
      OPT_ON[$i]=1
    else
      warn "Skip out-of-range: $part"
    fi
  done
  enforce_opt_deps
}

write_opt_menu() {
  local i=0 mark lock sel=""
  echo ""
  while [ $i -lt ${#OPT_IDS[@]} ]; do
    mark=" "
    [ "${OPT_ON[$i]}" = "1" ] && mark="x"
    lock=""
    [ "${OPT_LOCKED[$i]}" = "1" ] && lock=" (required)"
    printf '  [%s] %d. %s%s\n' "$mark" "$((i + 1))" "${OPT_LABEL[$i]}" "$lock"
    [ "${OPT_ON[$i]}" = "1" ] && sel="$sel ${OPT_IDS[$i]}"
    i=$((i + 1))
  done
  echo "  --> selected:$sel"
}

show_plan() {
  local data="$1" enabled="" i files="docker-compose.yml" okans
  title "4) Summary"
  i=0
  while [ $i -lt ${#OPT_IDS[@]} ]; do
    [ "${OPT_ON[$i]}" = "1" ] && enabled="$enabled ${OPT_IDS[$i]}"
    i=$((i + 1))
  done
  info "Profiles:${enabled}"
  info "Project:  $PROJECT_NAME"
  info "Data:     $data"
  if [ "$(opt_get binary)" = "1" ]; then
    info "Images:   local build"
  else
    info "Images:   ghcr.io/direktorbani/datasafe-storage-server:$TAG"
  fi
  [ "$(opt_get data)" = "1" ] && files="$files + docker-compose.local-data.yml"
  [ "$(opt_get binary)" = "1" ] && files="$files + deploy/compose/docker-compose.local-binary.yml"
  info "Compose:  $files"
  echo ""
  if [ "$YES" = "0" ]; then
    printf '  Proceed with installation? [Y/n] '
    read -r okans
    case "$okans" in N|n|No|no) err "Aborted by user."; exit 1 ;; esac
  fi
}

set_env_key() {
  local file="$1" key="$2" value="$3" tmp
  tmp="$(mktemp)"
  if grep -q "^[[:space:]]*${key}=" "$file" 2>/dev/null; then
    # Avoid in-place sed -i portability issues
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        ${key}=*|[[:space:]]${key}=*) printf '%s=%s\n' "$key" "$value" ;;
        *) printf '%s\n' "$line" ;;
      esac
    done <"$file" >"$tmp"
    mv "$tmp" "$file"
  else
    printf '%s=%s\n' "$key" "$value" >>"$file"
    rm -f "$tmp"
  fi
}

find_local_server_image() {
  local img candidates="datasafe-storage-server:latest
ghcr.io/direktorbani/datasafe-storage-server:v1.1.0
ghcr.io/direktorbani/datasafe-storage-server:v1.0.3
ghcr.io/direktorbani/datasafe-storage-server:v1.0.2
ghcr.io/direktorbani/datasafe-storage-server:v1.0.0
datasafe-storage-server:v1.0.0"
  while IFS= read -r img; do
    [ -z "$img" ] && continue
    if docker image inspect "$img" >/dev/null 2>&1; then
      printf '%s\n' "$img"
      return 0
    fi
  done <<EOF
$candidates
EOF
  img="$(docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null | grep 'datasafe-storage-server' | grep -v '<none>' | head -n1 || true)"
  [ -n "$img" ] && printf '%s\n' "$img" && return 0
  return 1
}

ensure_env() {
  local data="$1" local_img=""
  if [ ! -f .env ]; then
    cp .env.example .env
    ok "Created .env from .env.example"
  else
    ok ".env already exists (keys patched)"
  fi
  set_env_key .env DATASAFE_DATA_ROOT "$data"
  [ "$(opt_get postgres)" = "1" ] && set_env_key .env STORAGE_METADATA_BACKEND postgres
  if [ "$(opt_get binary)" = "1" ]; then
    if local_img="$(find_local_server_image)"; then
      set_env_key .env DATASAFE_SERVER_IMAGE "$local_img"
      ok "Binary mode base image: $local_img (no docker build)"
    else
      err "Binary mode needs a local datasafe-storage-server image (compose must not rebuild)."
      err "Pull one first, or build once with network access, then re-run with option 5."
      exit 1
    fi
  else
    set_env_key .env DATASAFE_SERVER_IMAGE "ghcr.io/direktorbani/datasafe-storage-server:$TAG"
    set_env_key .env DATASAFE_CONSOLE_IMAGE "ghcr.io/direktorbani/datasafe-console:$TAG"
  fi
}

make_data_dirs() {
  mkdir -p "$1/storage" "$1/postgres"
  ok "Data directories under $1"
}

build_local() {
  title "Build from source"
  info "Building Linux storage-server..."
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
    -o deploy/docker/storage-server-linux ./cmd/storage-server
  ok "deploy/docker/storage-server-linux"
  if [ ! -f web/console/dist/index.html ]; then
    info "Building web console..."
    (cd web/console && (npm ci || npm install) && npm run build)
  else
    ok "web/console/dist already present"
  fi
}

start_identity() {
  title "Identity lab"
  if [ -f scripts/start-ldap-test.cmd ] && command -v cmd.exe >/dev/null 2>&1; then
    cmd.exe /c scripts\\start-ldap-test.cmd || true
    cmd.exe /c scripts\\start-keycloak-test.cmd || true
  else
    warn "Identity lab scripts are Windows .cmd oriented on this host."
    warn "See docs/integrations/ldap-keycloak-standalone.md"
  fi
}

compose_up() {
  title "5) Apply"
  local args=(compose -p "$PROJECT_NAME" -f docker-compose.yml)
  [ "$(opt_get data)" = "1" ] && args+=(-f docker-compose.local-data.yml)
  [ "$(opt_get binary)" = "1" ] && args+=(-f deploy/compose/docker-compose.local-binary.yml)
  [ "$(opt_get postgres)" = "1" ] && args+=(--profile postgres)

  local services=(storage-server caddy)
  [ "$(opt_get postgres)" = "1" ] && services=(postgres "${services[@]}")
  [ "$(opt_get monitoring)" = "1" ] && services+=(prometheus grafana)

  if [ "$DRY_RUN" = "1" ]; then
    info "DRY-RUN docker ${args[*]} config"
    docker "${args[@]}" config --quiet
    ok "Compose config valid"
    info "Would up -d: ${services[*]}"
    return 0
  fi

  if [ "$(opt_get binary)" = "1" ]; then
    info "Using --no-build (local binary overlay)"
    info "docker ${args[*]} up -d --no-build ${services[*]}"
    docker "${args[@]}" up -d --no-build "${services[@]}"
  else
    info "docker ${args[*]} up -d ${services[*]}"
    docker "${args[@]}" up -d "${services[@]}"
  fi
  ok "Compose up requested"
}

wait_healthy() {
  title "6) Verify"
  local i url="http://127.0.0.1:8080/healthz"
  info "Waiting for $url ..."
  i=1
  while [ "$i" -le 90 ]; do
    if command -v curl >/dev/null 2>&1 && curl -fsS "$url" >/dev/null 2>&1; then
      ok "Console edge healthy"
      return 0
    fi
    sleep 2
    i=$((i + 1))
  done
  warn "healthz not ready yet - check docker compose -p datasafe ps"
}

show_done() {
  title "Done"
  echo ""
  echo "  URLs"
  echo "  Console:  http://localhost:8080"
  echo "  S3 API:   http://localhost:9000"
  [ "$(opt_get monitoring)" = "1" ] && echo "  Grafana:  http://localhost:3000"
  if [ "$(opt_get identity)" = "1" ]; then
    echo "  Keycloak: http://localhost:8180/admin"
    echo "  LDAP:     ldap://localhost:389"
  fi
  echo ""
  echo "  Pre-created users / passwords"
  echo "  Console (local):     admin / admin          (change on first login)"
  [ "$(opt_get monitoring)" = "1" ] && echo "  Grafana:             admin / admin"
  if [ "$(opt_get identity)" = "1" ]; then
    echo "  Keycloak admin:      admin / admin"
    echo "  LDAP bind DN:        cn=admin,dc=datasafe,dc=local / ldapadmin"
    echo "  LDAP test user:      ldapuser / password"
    echo "  LDAP test admin:     ldapadmin / password"
    echo "  SSO / Keycloak user: ssouser / password"
  fi
  echo ""
  echo "  Next: open console → change password → finish setup wizard."
  echo "  Docs: docs/getting-started/en/onboarding.md"
  echo ""
}

# --- main ---
show_detect
ensure_docker
DATA="$(default_data_root)"

INSTALL_MODE="single"
if [ "$CLUSTER" = "1" ]; then
  INSTALL_MODE="cluster"
elif [ "$YES" != "1" ]; then
  echo ""
  echo "  Install mode:"
  echo "    [1] Single-node (this machine / local Docker Compose)"
  echo "    [2] Cluster (≥3 Linux VMs; Wave 1 = inventory + preflight)"
  echo "    Q   quit"
  while true; do
    printf '  Select: '
    read -r a || true
    case "${a:-}" in
      1) INSTALL_MODE="single"; break ;;
      2) INSTALL_MODE="cluster"; break ;;
      Q|q) err "Aborted by user."; exit 1 ;;
      *) warn "Enter 1, 2, or Q" ;;
    esac
  done
fi

if [ "$INSTALL_MODE" = "cluster" ]; then
  title "Cluster mode (Wave 1+2 DryRun)"
  info "Wave 2 DryRun renders Patroni/etcd/HAProxy/keepalived/NFS; live --apply is later."
  if [ "$YES" = "1" ] && [ "$DRY_RUN" != "1" ]; then
    err "Non-interactive Cluster Apply is not ready. Use: ./install.sh --cluster --dry-run --yes"
    exit 1
  fi
  if [ "$YES" = "1" ] && [ "$DRY_RUN" = "1" ]; then
    warn "DryRun + --yes: running bash Wave 1+2 asserts (PowerShell optional extra)"
    if [ -f "$ROOT/scripts/tests/cluster-installer-w1.sh" ]; then
      bash "$ROOT/scripts/tests/cluster-installer-w1.sh" || exit 1
    fi
    bash "$ROOT/scripts/tests/cluster-installer-w2.sh" || exit 1
    if command -v pwsh >/dev/null 2>&1; then
      pwsh -NoProfile -File "$ROOT/scripts/tests/cluster-installer-w1.ps1" || exit 1
      pwsh -NoProfile -File "$ROOT/scripts/tests/cluster-installer-w2.ps1" || exit 1
    elif command -v powershell >/dev/null 2>&1; then
      powershell -NoProfile -File "$ROOT/scripts/tests/cluster-installer-w1.ps1" || exit 1
      powershell -NoProfile -File "$ROOT/scripts/tests/cluster-installer-w2.ps1" || exit 1
    fi
    title "DryRun complete"
    ok "Cluster Wave 1+2 validation OK"
    exit 0
  fi
  wiz_args=()
  [ "$DRY_RUN" = "1" ] && wiz_args+=(--dry-run)
  bash "$ROOT/scripts/cluster/cluster_wizard_w1.sh" "${wiz_args[@]}"
  exit 0
fi

if [ "$YES" = "1" ]; then
  if [ -n "$PROFILES" ]; then set_profiles "$PROFILES"
  else set_profiles "core,postgres,monitoring,data"; fi
else
  [ -n "$PROFILES" ] && set_profiles "$PROFILES"
  show_menu
  echo ""
  echo "  Where to store data on the host (Postgres + object files):"
  printf '  Data root [%s]: ' "$DATA"
  read -r dr
  [ -n "${dr:-}" ] && DATA="$dr"
fi

show_plan "$DATA"
if [ "$DRY_RUN" = "1" ]; then
  warn "DryRun: skip .env write, builds, sidecars - validating compose only"
  compose_up
  title "DryRun complete"
  ok "Configuration validated"
  exit 0
fi
ensure_env "$DATA"
[ "$(opt_get data)" = "1" ] && make_data_dirs "$DATA"
[ "$(opt_get binary)" = "1" ] && build_local
[ "$(opt_get identity)" = "1" ] && start_identity
compose_up
wait_healthy
show_done
