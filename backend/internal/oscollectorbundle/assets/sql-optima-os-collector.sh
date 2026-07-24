#!/usr/bin/env bash
# SQL Optima OS Collector — host telemetry for PostgreSQL (Linux).
# SPDX-License-Identifier: MIT
set -euo pipefail

VERSION="2.1.0"
PROC_ROOT="${PROC_ROOT:-/proc}"
CPU_SAMPLE_SEC="${CPU_SAMPLE_SEC:-1}"

# Replaced when the bundle is built from SQL Optima UI (see bundled-config.env).
BUNDLE_BACKEND_URL='__BUNDLE_BACKEND_URL__'
BUNDLE_INSTANCE_NAME='__BUNDLE_INSTANCE_NAME__'
BUNDLE_SERVER_ID='__BUNDLE_SERVER_ID__'
BUNDLE_APP_URL='__BUNDLE_APP_URL__'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BACKEND_URL="${SQL_OPTIMA_BACKEND_URL:-}"
API_KEY="${SQL_OPTIMA_API_KEY:-}"
INSTANCE_NAME="${SQL_OPTIMA_INSTANCE_NAME:-}"
SERVER_ID="${SQL_OPTIMA_SERVER_ID:-}"
APP_URL="${SQL_OPTIMA_APP_URL:-}"
INTERVAL_SEC=30
CRON_INTERVAL_MIN="${SQL_OPTIMA_CRON_INTERVAL_MIN:-5}"
ONCE_MODE=0

fail_streak=0
shutdown=0

log() {
  printf '%s %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" "$*" >&2
}

die() {
  log "FATAL: $*"
  exit 1
}

bundle_placeholder() {
  [[ "$1" =~ ^__BUNDLE_.*__$ ]]
}

apply_bundle_defaults() {
  if [[ -f "${SCRIPT_DIR}/bundled-config.env" ]]; then
    # shellcheck source=/dev/null
    source "${SCRIPT_DIR}/bundled-config.env"
  fi
  if [[ -z "$BACKEND_URL" ]] && ! bundle_placeholder "$BUNDLE_BACKEND_URL"; then
    BACKEND_URL="$BUNDLE_BACKEND_URL"
  fi
  if [[ -z "$INSTANCE_NAME" ]] && ! bundle_placeholder "$BUNDLE_INSTANCE_NAME"; then
    INSTANCE_NAME="$BUNDLE_INSTANCE_NAME"
  fi
  if [[ -z "$SERVER_ID" ]] && ! bundle_placeholder "$BUNDLE_SERVER_ID"; then
    SERVER_ID="$BUNDLE_SERVER_ID"
  fi
  if [[ -z "$APP_URL" ]] && ! bundle_placeholder "$BUNDLE_APP_URL"; then
    APP_URL="$BUNDLE_APP_URL"
  fi
}

require_deps() {
  local missing=()
  command -v curl >/dev/null 2>&1 || missing+=("curl")
  command -v jq >/dev/null 2>&1 || missing+=("jq")
  command -v awk >/dev/null 2>&1 || missing+=("awk")
  command -v date >/dev/null 2>&1 || missing+=("date")
  if ((${#missing[@]} > 0)); then
    die "missing required commands: ${missing[*]}"
  fi
  [[ -r "${PROC_ROOT}/meminfo" ]] || die "cannot read ${PROC_ROOT}/meminfo (Linux /proc required)"
}

parse_interval() {
  local raw="${1:-30}"
  if [[ "$raw" =~ ^[0-9]+$ ]]; then
    INTERVAL_SEC="$raw"
    return
  fi
  if [[ "$raw" =~ ^([0-9]+)s$ ]]; then
    INTERVAL_SEC="${BASH_REMATCH[1]}"
    return
  fi
  if [[ "$raw" =~ ^([0-9]+)m$ ]]; then
    INTERVAL_SEC=$((BASH_REMATCH[1] * 60))
    return
  fi
  if [[ "$raw" =~ ^([0-9]+)h$ ]]; then
    INTERVAL_SEC=$((BASH_REMATCH[1] * 3600))
    return
  fi
  die "invalid interval: $raw (use e.g. 30, 30s, 5m)"
}

validate_config() {
  [[ -n "$BACKEND_URL" ]] || die "backend URL required: -url or SQL_OPTIMA_BACKEND_URL"
  [[ -n "$INSTANCE_NAME" ]] || die "instance name required: -instance or SQL_OPTIMA_INSTANCE_NAME"
  [[ -n "$API_KEY" ]] || die "API key required: -key or SQL_OPTIMA_API_KEY (admin JWT)"
  if [[ "$API_KEY" == "PASTE_ADMIN_JWT_HERE" ]]; then
    die "set SQL_OPTIMA_API_KEY (admin JWT) in bundled-config.env or run: $0 install"
  fi
  if ((INTERVAL_SEC < 10)); then
    die "interval must be >= 10s (got ${INTERVAL_SEC}s)"
  fi
}

meminfo_kb() {
  local key="$1"
  awk -v k="$key" '
    $1 == k ":" { gsub(/ kB/, "", $2); print $2; exit }
  ' "${PROC_ROOT}/meminfo" 2>/dev/null || echo 0
}

read_memory_metrics() {
  local mem_total_kb mem_avail_kb mem_free_kb cached_kb sreclaim_kb buffers_kb shmem_kb slab_kb dirty_kb writeback_kb
  local swap_total_kb swap_free_kb

  mem_total_kb=$(meminfo_kb MemTotal)
  mem_avail_kb=$(meminfo_kb MemAvailable)
  mem_free_kb=$(meminfo_kb MemFree)
  cached_kb=$(meminfo_kb Cached)
  sreclaim_kb=$(meminfo_kb SReclaimable)
  buffers_kb=$(meminfo_kb Buffers)
  shmem_kb=$(meminfo_kb Shmem)
  slab_kb=$(meminfo_kb Slab)
  dirty_kb=$(meminfo_kb Dirty)
  writeback_kb=$(meminfo_kb Writeback)
  swap_total_kb=$(meminfo_kb SwapTotal)
  swap_free_kb=$(meminfo_kb SwapFree)

  cached_kb=$((cached_kb + sreclaim_kb))

  MEM_TOTAL_BYTES=$((mem_total_kb * 1024))
  MEM_FREE_BYTES=$((mem_free_kb * 1024))
  MEM_BUFFERS_BYTES=$((buffers_kb * 1024))
  MEM_CACHED_BYTES=$((cached_kb * 1024))
  MEM_SHARED_BYTES=$((shmem_kb * 1024))
  MEM_SLAB_BYTES=$((slab_kb * 1024))
  MEM_DIRTY_BYTES=$((dirty_kb * 1024))
  MEM_WRITEBACK_BYTES=$((writeback_kb * 1024))

  if [[ "$mem_avail_kb" -gt 0 ]]; then
    MEM_AVAILABLE_BYTES=$((mem_avail_kb * 1024))
  else
    MEM_AVAILABLE_BYTES=$(((mem_free_kb + cached_kb) * 1024))
  fi

  MEM_USED_BYTES=$((MEM_TOTAL_BYTES - MEM_FREE_BYTES - MEM_BUFFERS_BYTES - MEM_CACHED_BYTES))

  SWAP_TOTAL_BYTES=$((swap_total_kb * 1024))
  SWAP_FREE_BYTES=$((swap_free_kb * 1024))
  SWAP_USED_BYTES=$((SWAP_TOTAL_BYTES - SWAP_FREE_BYTES))
}

read_cpu_stat_line() {
  awk '/^cpu / {
    print $2,$3,$4,$5,$6,$7,$8,$9,$10
    exit
  }' "${PROC_ROOT}/stat"
}

cpu_delta_percents() {
  local u1 n1 s1 i1 io1 irq1 soft1 st1
  local u2 n2 s2 i2 io2 irq2 soft2 st2
  read -r u1 n1 s1 i1 io1 irq1 soft1 st1 _ <<<"$1"
  read -r u2 n2 s2 i2 io2 irq2 soft2 st2 _ <<<"$2"

  awk -v u1="$u1" -v n1="$n1" -v s1="$s1" -v i1="$i1" -v io1="$io1" -v irq1="$irq1" -v soft1="$soft1" -v st1="$st1" \
      -v u2="$u2" -v n2="$n2" -v s2="$s2" -v i2="$i2" -v io2="$io2" -v irq2="$irq2" -v soft2="$soft2" -v st2="$st2" '
    function delta(a, b) { return (a >= b) ? a - b : 0 }
    BEGIN {
      du = delta(u2, u1); dn = delta(n2, n1); ds = delta(s2, s1)
      di = delta(i2, i1); dio = delta(io2, io1)
      dirq = delta(irq2, irq1); dsoft = delta(soft2, soft1); dsteal = delta(st2, st1)
      total = du + ds + di + dio + dn + dirq + dsoft + dsteal
      if (total <= 0) {
        print 0, 0, 100, 0
      } else {
        printf "%.6f %.6f %.6f %.6f\n", (du+dn)/total*100, (ds+dirq+dsoft)/total*100, di/total*100, dio/total*100
      }
    }'
}

sample_cpu_pct() {
  local t1 t2
  t1=$(read_cpu_stat_line)
  if [[ -z "$t1" ]]; then
    CPU_USER_PCT=0; CPU_SYSTEM_PCT=0; CPU_IDLE_PCT=100; CPU_IOWAIT_PCT=0
    return
  fi
  if [[ -z "${SQL_OPTIMA_SKIP_CPU_SAMPLE:-}" ]]; then
    sleep "$CPU_SAMPLE_SEC"
  fi
  t2=$(read_cpu_stat_line)
  if [[ -z "$t2" ]]; then
    CPU_USER_PCT=0; CPU_SYSTEM_PCT=0; CPU_IDLE_PCT=100; CPU_IOWAIT_PCT=0
    return
  fi
  read -r CPU_USER_PCT CPU_SYSTEM_PCT CPU_IDLE_PCT CPU_IOWAIT_PCT <<<"$(cpu_delta_percents "$t1" "$t2")"
}

read_loadavg() {
  read -r LOAD_1M LOAD_5M LOAD_15M _ <"${PROC_ROOT}/loadavg"
}

read_cpu_cores() {
  if [[ -n "${SQL_OPTIMA_CPU_CORES:-}" ]]; then
    CPU_CORES="$SQL_OPTIMA_CPU_CORES"
    return
  fi
  if [[ -f "${PROC_ROOT}/cpuinfo" ]]; then
    CPU_CORES=$(awk '/^processor[[:space:]]*:/ { c++ } END { print c+0 }' "${PROC_ROOT}/cpuinfo")
  elif command -v nproc >/dev/null 2>&1; then
    CPU_CORES=$(nproc)
  else
    CPU_CORES=1
  fi
  [[ "$CPU_CORES" -gt 0 ]] || CPU_CORES=1
}

is_postgres_comm() {
  local name="$1"
  [[ "$name" == "postgres" || "$name" == postgres:* ]]
}

collect_postgres_proc() {
  local pid comm status rss_kb vsz_kb
  POSTGRES_RSS_BYTES=0
  POSTGRES_VSZ_BYTES=0
  BACKEND_COUNT=0

  for pid_dir in "${PROC_ROOT}"/[0-9]*; do
    [[ -d "$pid_dir" ]] || continue
    pid="${pid_dir##*/}"
    [[ "$pid" =~ ^[0-9]+$ ]] || continue
    comm=$(tr -d '\n' <"${pid_dir}/comm" 2>/dev/null) || continue
    is_postgres_comm "$comm" || continue
    status=$(<"${pid_dir}/status") || continue
    rss_kb=$(awk '/^VmRSS:/ { print $2; exit }' <<<"$status")
    vsz_kb=$(awk '/^VmSize:/ { print $2; exit }' <<<"$status")
    POSTGRES_RSS_BYTES=$((POSTGRES_RSS_BYTES + rss_kb * 1024))
    POSTGRES_VSZ_BYTES=$((POSTGRES_VSZ_BYTES + vsz_kb * 1024))
    BACKEND_COUNT=$((BACKEND_COUNT + 1))
  done
}

read_hostname() {
  if [[ -r "${PROC_ROOT}/sys/kernel/hostname" ]]; then
    HOSTNAME=$(<"${PROC_ROOT}/sys/kernel/hostname")
    HOSTNAME="${HOSTNAME%%$'\n'*}"
  elif command -v hostname >/dev/null 2>&1; then
    HOSTNAME=$(hostname -f 2>/dev/null || hostname)
  else
    HOSTNAME="unknown"
  fi
}

build_json_payload() {
  local ts="$1"
  jq -n \
    --arg ts "$ts" \
    --arg hostname "$HOSTNAME" \
    --arg instance_name "$INSTANCE_NAME" \
    --argjson total_bytes "$MEM_TOTAL_BYTES" \
    --argjson available_bytes "$MEM_AVAILABLE_BYTES" \
    --argjson used_bytes "$MEM_USED_BYTES" \
    --argjson free_bytes "$MEM_FREE_BYTES" \
    --argjson cached_bytes "$MEM_CACHED_BYTES" \
    --argjson buffers_bytes "$MEM_BUFFERS_BYTES" \
    --argjson shared_bytes "$MEM_SHARED_BYTES" \
    --argjson slab_bytes "$MEM_SLAB_BYTES" \
    --argjson swap_total_bytes "$SWAP_TOTAL_BYTES" \
    --argjson swap_used_bytes "$SWAP_USED_BYTES" \
    --argjson swap_free_bytes "$SWAP_FREE_BYTES" \
    --argjson dirty_bytes "$MEM_DIRTY_BYTES" \
    --argjson writeback_bytes "$MEM_WRITEBACK_BYTES" \
    --argjson cpu_user_pct "$CPU_USER_PCT" \
    --argjson cpu_system_pct "$CPU_SYSTEM_PCT" \
    --argjson cpu_idle_pct "$CPU_IDLE_PCT" \
    --argjson cpu_iowait_pct "$CPU_IOWAIT_PCT" \
    --argjson load_1m "$LOAD_1M" \
    --argjson load_5m "$LOAD_5M" \
    --argjson load_15m "$LOAD_15M" \
    --argjson cpu_cores "$CPU_CORES" \
    --argjson postgres_rss_bytes "$POSTGRES_RSS_BYTES" \
    --argjson postgres_vsz_bytes "$POSTGRES_VSZ_BYTES" \
    --argjson backend_count "$BACKEND_COUNT" \
    '{
      ts: $ts,
      hostname: $hostname,
      instance_name: $instance_name,
      total_bytes: $total_bytes,
      available_bytes: $available_bytes,
      used_bytes: $used_bytes,
      free_bytes: $free_bytes,
      cached_bytes: $cached_bytes,
      buffers_bytes: $buffers_bytes,
      shared_bytes: $shared_bytes,
      slab_bytes: $slab_bytes,
      swap_total_bytes: $swap_total_bytes,
      swap_used_bytes: $swap_used_bytes,
      swap_free_bytes: $swap_free_bytes,
      dirty_bytes: $dirty_bytes,
      writeback_bytes: $writeback_bytes,
      cpu_user_pct: $cpu_user_pct,
      cpu_system_pct: $cpu_system_pct,
      cpu_idle_pct: $cpu_idle_pct,
      cpu_iowait_pct: $cpu_iowait_pct,
      load_1m: $load_1m,
      load_5m: $load_5m,
      load_15m: $load_15m,
      cpu_cores: $cpu_cores,
      postgres_rss_bytes: $postgres_rss_bytes,
      postgres_vsz_bytes: $postgres_vsz_bytes,
      backend_count: $backend_count
    }'
}

collect_metrics() {
  local ts
  ts=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
  read_hostname
  read_memory_metrics
  sample_cpu_pct
  read_loadavg
  read_cpu_cores
  collect_postgres_proc
  build_json_payload "$ts"
}

push_metrics() {
  local payload="$1"
  local http_code resp_file
  resp_file=$(mktemp)
  http_code=$(curl -sS -o "$resp_file" -w '%{http_code}' --max-time 10 \
    -X POST "$BACKEND_URL" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${API_KEY}" \
    -d "$payload") || { rm -f "$resp_file"; return 1; }
  if [[ "$http_code" -lt 200 ]] || [[ "$http_code" -ge 300 ]]; then
    local detail
    detail=$(tr '\n' ' ' <"$resp_file" | head -c 240)
    rm -f "$resp_file"
    if [[ -n "$detail" ]]; then
      log "push failed: HTTP ${http_code} — ${detail}"
    else
      log "push failed: HTTP ${http_code}"
    fi
    return 1
  fi
  rm -f "$resp_file"
  return 0
}

run_once() {
  local payload
  payload=$(collect_metrics) || { log "collect failed"; return; }
  if [[ -n "${SQL_OPTIMA_COLLECT_ONLY:-}" ]]; then
    printf '%s\n' "$payload"
    exit 0
  fi
  if push_metrics "$payload"; then
    if ((fail_streak > 0)); then
      log "push recovered after ${fail_streak} failure(s)"
    fi
    fail_streak=0
    return 0
  fi
  fail_streak=$((fail_streak + 1))
  if ((ONCE_MODE)); then
    return 1
  fi
  local backoff=$((fail_streak * 5))
  ((backoff > 120)) && backoff=120
  log "push failed (${fail_streak}), backoff ${backoff}s"
  sleep "$backoff"
  return 1
}

on_signal() {
  shutdown=1
  log "shutting down"
}

usage() {
  cat <<EOF
sql-optima-os-collector ${VERSION} — PostgreSQL host telemetry (Linux)

Usage:
  $(basename "$0") install [--systemd]   Install config + cron (default) or systemd daemon
  $(basename "$0") --once               Collect and push once (for cron)
  $(basename "$0") [options]            Run continuously (loop every interval)

  $(basename "$0") install
    Reads bundled-config.env from this folder (from SQL Optima download).
    Prompts for admin JWT if needed, installs cron job (every ${CRON_INTERVAL_MIN} min by default).

  $(basename "$0") install --systemd
    Use systemd instead of cron (30s interval loop; requires root).

Options (run / daemon mode):
  -url URL          Backend URL (POST /api/os/metrics)
  -key TOKEN        Admin JWT (Authorization: Bearer)
  -instance NAME    SQL Optima instance name
  -interval DUR     Loop interval (default 30s, minimum 10s)

Bundled config: SQL_OPTIMA_* in bundled-config.env (auto-filled from UI download).

Requires: bash, curl, jq, awk, Linux /proc
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -url) BACKEND_URL="$2"; shift 2 ;;
      -key) API_KEY="$2"; shift 2 ;;
      -instance) INSTANCE_NAME="$2"; shift 2 ;;
      -interval) parse_interval "$2"; shift 2 ;;
      --once) ONCE_MODE=1; shift ;;
      -version) echo "sql-optima-os-collector ${VERSION}"; exit 0 ;;
      -h|--help|help) usage; exit 0 ;;
      *) die "unknown argument: $1" ;;
    esac
  done
}

prompt_api_key_if_needed() {
  if [[ -n "$API_KEY" && "$API_KEY" != "PASTE_ADMIN_JWT_HERE" ]]; then
    return
  fi
  if [[ ! -t 0 ]]; then
    die "SQL_OPTIMA_API_KEY not set; edit bundled-config.env or run install interactively"
  fi
  echo -n "Admin JWT for SQL Optima (Bearer token): " >&2
  read -rs API_KEY
  echo >&2
  [[ -n "$API_KEY" ]] || die "API key is required"
}

write_env_file() {
  local dest="$1"
  local dir
  dir=$(dirname "$dest")
  mkdir -p "$dir"
  {
    echo "# SQL Optima OS Collector — generated by install"
    # shellcheck disable=SC2034
    printf 'SQL_OPTIMA_BACKEND_URL=%q\n' "$BACKEND_URL"
    printf 'SQL_OPTIMA_API_KEY=%q\n' "$API_KEY"
    printf 'SQL_OPTIMA_INSTANCE_NAME=%q\n' "$INSTANCE_NAME"
    printf 'SQL_OPTIMA_SERVER_ID=%q\n' "$SERVER_ID"
    printf 'SQL_OPTIMA_APP_URL=%q\n' "$APP_URL"
    printf 'SQL_OPTIMA_CRON_INTERVAL_MIN=%q\n' "$CRON_INTERVAL_MIN"
  } >"$dest"
  chmod 600 "$dest"
}

install_paths() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    INSTALL_BIN="/usr/local/bin/sql-optima-os-collector.sh"
    ENV_FILE="/etc/sql-optima/os-collector.env"
    LOG_FILE="/var/log/sql-optima-os-collector.log"
    RUN_USER="${SQL_OPTIMA_RUN_USER:-postgres}"
  else
    INSTALL_BIN="${HOME}/.local/bin/sql-optima-os-collector.sh"
    ENV_FILE="${HOME}/.config/sql-optima/os-collector.env"
    LOG_FILE="${HOME}/.local/share/sql-optima/os-collector.log"
    RUN_USER="$(id -un)"
  fi
}

cmd_install() {
  local use_systemd=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --systemd) use_systemd=1; shift ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown install option: $1" ;;
    esac
  done

  apply_bundle_defaults
  require_deps
  prompt_api_key_if_needed
  [[ -n "$BACKEND_URL" && -n "$INSTANCE_NAME" ]] || die "missing URL or instance in bundled-config.env"

  install_paths
  mkdir -p "$(dirname "$INSTALL_BIN")" "$(dirname "$LOG_FILE")"
  cp -f "${SCRIPT_DIR}/$(basename "$0")" "$INSTALL_BIN"
  chmod 755 "$INSTALL_BIN"
  write_env_file "$ENV_FILE"

  if ((use_systemd)) && command -v systemctl >/dev/null 2>&1 && [[ "${EUID:-0}" -eq 0 ]]; then
    install_systemd_unit "$ENV_FILE" "$RUN_USER"
    log "installed systemd service (interval loop ~30s)"
    log "  systemctl status sql-optima-os-collector"
    return
  fi

  install_cron_job "$INSTALL_BIN" "$ENV_FILE" "$LOG_FILE"
  log "installed cron schedule (every ${CRON_INTERVAL_MIN} minutes)"
  log "  config: ${ENV_FILE}"
  log "  log:    ${LOG_FILE}"
  log "  test:   ENV_FILE=${ENV_FILE} ${INSTALL_BIN} --once"
  if ((use_systemd)) && [[ "${EUID:-0}" -ne 0 ]]; then
    log "note: --systemd skipped (run install as root for systemd)"
  fi
}

install_systemd_unit() {
  local env_file="$1"
  local run_user="$2"
  cat >/etc/systemd/system/sql-optima-os-collector.service <<EOF
[Unit]
Description=SQL Optima OS Collector
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=${run_user}
EnvironmentFile=${env_file}
ExecStart=${INSTALL_BIN} -interval 30s
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable --now sql-optima-os-collector.service
}

install_cron_job() {
  local bin="$1"
  local env_file="$2"
  local log_file="$3"
  local cron_line cron_tmp
  if ! command -v crontab >/dev/null 2>&1; then
    die "crontab not found; use: $0 install --systemd (as root) or run manually: $bin --once"
  fi
  ((CRON_INTERVAL_MIN < 1)) && CRON_INTERVAL_MIN=5
  if ((CRON_INTERVAL_MIN >= 60)); then
    cron_line="0 * * * *"
  elif ((60 % CRON_INTERVAL_MIN == 0)); then
    cron_line="*/${CRON_INTERVAL_MIN} * * * *"
  else
    cron_line="*/${CRON_INTERVAL_MIN} * * * *"
  fi
  cron_line="${cron_line} /bin/bash -c 'set -a; source \"${env_file}\"; set +a; \"${bin}\" --once' >>\"${log_file}\" 2>&1"

  cron_tmp=$(mktemp)
  crontab -l 2>/dev/null | grep -v 'sql-optima-os-collector' >"$cron_tmp" || true
  echo "$cron_line" >>"$cron_tmp"
  crontab "$cron_tmp"
  rm -f "$cron_tmp"
}

main_daemon() {
  parse_args "$@"
  apply_bundle_defaults
  require_deps
  validate_config
  read_hostname
  log "sql-optima-os-collector ${VERSION} instance=${INSTANCE_NAME} server_id=${SERVER_ID:-n/a} host=${HOSTNAME} interval=${INTERVAL_SEC}s"

  trap on_signal INT TERM

  run_once
  while ((shutdown == 0)); do
    sleep "$INTERVAL_SEC" || break
    ((shutdown != 0)) && break
    run_once
  done
}

main_once() {
  # Invoked via top-level "--once" (cron); parse_args would not see that flag after the case shift.
  ONCE_MODE=1
  parse_args "$@"
  apply_bundle_defaults
  require_deps
  validate_config
  run_once
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  case "${1:-}" in
    install) shift; cmd_install "$@"; exit 0 ;;
    --once) shift; main_once "$@"; exit $? ;;
    -h|--help|help) usage; exit 0 ;;
    -version) echo "sql-optima-os-collector ${VERSION}"; exit 0 ;;
  esac
  main_daemon "$@"
fi
