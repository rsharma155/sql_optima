#!/usr/bin/env bash
# SQL Optima - one-command local quick start (development / evaluation).
set -euo pipefail

cd "$(dirname "$0")"
# shellcheck source=scripts/dev-setup-lib.sh
source "$(dirname "$0")/scripts/dev-setup-lib.sh"

NO_BROWSER=0
if [[ "${SQL_OPTIMA_NO_BROWSER:-}" == "1" ]]; then
  NO_BROWSER=1
fi
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-browser) NO_BROWSER=1; shift ;;
    *) echo "[sql-optima] Unknown option: $1" >&2; exit 1 ;;
  esac
done

if [[ ! -f ../backend/go.mod ]]; then
  echo "[sql-optima] ERROR: backend/ is missing. Use the full repo (not only docker/)." >&2
  exit 1
fi

if [[ ! -f .env ]]; then
  if ! dev_is_interactive && [[ -z "${SQL_OPTIMA_SETUP_MODE:-}" ]]; then
    echo "[sql-optima] Non-interactive install: using Easy setup (no LAN DB exposure)."
    echo "[sql-optima] Override with SQL_OPTIMA_SETUP_MODE=custom and SQL_OPTIMA_EXPOSE_DB_LAN=1 if needed."
  fi
  dev_resolve_setup
  dev_write_env_file .env
  echo "[sql-optima] Created docker/.env (${DEV_SETUP_MODE} setup)."
elif [[ -f .env.dev ]] && [[ "${SQL_OPTIMA_FORCE_ENV_SETUP:-}" == "1" ]]; then
  dev_resolve_setup
  dev_write_env_file .env
  echo "[sql-optima] Regenerated docker/.env (${DEV_SETUP_MODE} setup)."
fi

if [[ -f .env ]]; then
  if grep -qE '^[[:space:]]*VAULT_TOKEN[[:space:]]*=[[:space:]]*root[[:space:]]*$' .env; then
    grep -vE '^[[:space:]]*VAULT_TOKEN[[:space:]]*=[[:space:]]*root[[:space:]]*$' .env > .env.tmp && mv .env.tmp .env
    echo "[sql-optima] Removed VAULT_TOKEN=root from .env (API reads token from Vault volume)."
  fi
fi

env_val() {
  local key="$1" default="$2"
  local line
  line="$(grep -E "^[[:space:]]*${key}=" .env 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '\r' || true)"
  [[ -n "${line:-}" ]] && echo "$line" || echo "$default"
}

print_timescale_access() {
  if [[ ! -f .env ]] || ! dev_env_expose_lan; then
    echo "[sql-optima] TimescaleDB is not published to the LAN (Easy setup or LAN disabled)."
    echo "[sql-optima] To inspect the DB: docker compose exec timescaledb psql -U dbmonitor -d dbmonitor_metrics"
    return 0
  fi
  local pub_port bind_ip db_user db_name
  pub_port="$(env_val DB_PUBLISH_PORT 5432)"
  bind_ip="$(env_val DB_PUBLISH_BIND "")"
  db_user="$(env_val DB_USER dbmonitor)"
  db_name="$(env_val DB_NAME dbmonitor_metrics)"
  echo "[sql-optima] TimescaleDB for DBeaver / psql (LAN-enabled, bind ${bind_ip:-LAN IP}):"
  echo "  Host: ${bind_ip}  Port: ${pub_port}  Database: ${db_name}  User: ${db_user}"
  echo "  Password: (the DB password you chose during setup, or see docker/.env DB_PASSWORD)"
  echo "  Same machine: localhost may not work if bound to LAN IP only - use Host ${bind_ip}."
  echo "  Allow TCP ${pub_port} in the host firewall for remote machines."
}

echo "[sql-optima] Starting stack (first run may take a few minutes to build)..."
if ! docker compose up --build -d; then
  echo ""
  echo "[sql-optima] docker compose failed. Service status:"
  docker compose ps -a 2>/dev/null || true
  echo ""
  echo "[sql-optima] Recent logs (schema-setup, api, vault):"
  docker compose logs schema-setup --tail 40 2>/dev/null || true
  docker compose logs api --tail 40 2>/dev/null || true
  docker compose logs vault --tail 40 2>/dev/null || true
  echo ""
  echo "[sql-optima] A line like 'successful mount: ... transit' in Vault logs usually means Vault is OK."
  echo "[sql-optima] Check the compose error above for the real cause (API build, schema-setup, DB password mismatch)."
  echo ""
  echo "[sql-optima] If the error mentions 'parent snapshot' / 'does not exist: not found', clear BuildKit cache:"
  echo "  docker builder prune -af"
  echo "  docker compose build --no-cache api"
  echo "  docker compose up -d"
  echo "[sql-optima] If schema-setup failed (auth / password) or after a partial install, reset volumes:"
  echo "  docker compose down -v"
  echo "  docker compose up --build -d"
  exit 1
fi

API_PORT="${API_PORT:-8080}"
if [[ -f .env ]]; then
  val="$(grep -E '^API_PORT=' .env 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '\r' || true)"
  [[ -n "${val:-}" ]] && API_PORT="$val"
fi

BASE_URL="http://127.0.0.1:${API_PORT}"
MAX_WAIT="${SQL_OPTIMA_WAIT_SEC:-900}"
INTERVAL=3
elapsed=0
last_progress=0

echo ""
echo "[sql-optima] Waiting for API at ${BASE_URL} (up to ${MAX_WAIT}s on first build)..."

wait_http_ok() {
  local url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -fsS --max-time 5 "$url" >/dev/null 2>&1
    return $?
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO- --timeout=5 "$url" >/dev/null 2>&1
    return $?
  fi
  return 1
}

while (( elapsed < MAX_WAIT )); do
  if wait_http_ok "${BASE_URL}/api/health"; then
    ready=0
    if command -v curl >/dev/null 2>&1; then
      if curl -fsS --max-time 5 "${BASE_URL}/api/setup/status" 2>/dev/null | grep -qE '"timescale_connected"[[:space:]]*:[[:space:]]*true'; then
        ready=1
      fi
    else
      ready=1
    fi
    [[ "$ready" -eq 1 ]] && break
  fi
  if (( elapsed - last_progress >= 15 )); then
    echo "[sql-optima] Still starting... (${elapsed}s)"
    last_progress=$elapsed
  fi
  sleep "$INTERVAL"
  elapsed=$((elapsed + INTERVAL))
done

if (( elapsed >= MAX_WAIT )); then
  echo "[sql-optima] ERROR: API did not become ready within ${MAX_WAIT}s." >&2
  echo "[sql-optima] Recent API logs:" >&2
  docker compose logs api --tail 50 2>/dev/null || true
  echo "[sql-optima] Try: docker compose ps && docker compose logs api" >&2
  exit 1
fi

echo "[sql-optima] API is ready (${elapsed}s)."

open_browser() {
  local url="$1"
  if [[ "$NO_BROWSER" -eq 1 ]]; then
    return 0
  fi
  if [[ "$(uname -s)" == "Darwin" ]]; then
    open "$url" 2>/dev/null && return 0
  fi
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" 2>/dev/null && return 0
  fi
  if grep -qi microsoft /proc/version 2>/dev/null && command -v cmd.exe >/dev/null 2>&1; then
    cmd.exe /c start "" "$url" 2>/dev/null && return 0
  fi
  return 1
}

DISPLAY_URL="http://localhost:${API_PORT}"
if [[ "$NO_BROWSER" -eq 0 ]]; then
  if open_browser "$DISPLAY_URL"; then
    echo "[sql-optima] Opened browser at ${DISPLAY_URL}"
  else
    echo "[sql-optima] Could not launch a browser automatically. Open: ${DISPLAY_URL}"
  fi
else
  echo "[sql-optima] Browser launch skipped (--no-browser)."
fi

echo ""
echo "[sql-optima] ${DISPLAY_URL}"
echo "[sql-optima] First visit: setup wizard - create your admin username and password."
echo "[sql-optima] Then: add PostgreSQL or SQL Server (or use the in-app guide for local HA test clusters)."
echo ""
print_timescale_access
echo ""
echo "Stop (keep data):  docker compose down"
echo "Stop + wipe data: docker compose down -v"
