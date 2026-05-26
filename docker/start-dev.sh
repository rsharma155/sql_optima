#!/usr/bin/env bash
# SQL Optima — one-command local quick start (development / evaluation).
set -euo pipefail

cd "$(dirname "$0")"

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
  cp .env.dev .env
  echo "[sql-optima] Created docker/.env from .env.dev (ready-to-run dev defaults)."
fi

if [[ -f .env ]]; then
  if grep -qE '^[[:space:]]*VAULT_TOKEN[[:space:]]*=[[:space:]]*root[[:space:]]*$' .env; then
    grep -vE '^[[:space:]]*VAULT_TOKEN[[:space:]]*=[[:space:]]*root[[:space:]]*$' .env > .env.tmp && mv .env.tmp .env
    echo "[sql-optima] Removed VAULT_TOKEN=root from .env (API reads token from Vault volume)."
  fi
fi

echo "[sql-optima] Starting stack (first run may take a few minutes to build)..."
if ! docker compose up --build -d; then
  echo ""
  echo "[sql-optima] docker compose failed. Vault is often the cause on first run or after a partial start."
  echo "[sql-optima] Vault logs:"
  docker compose logs vault --tail 80 2>/dev/null || true
  echo ""
  echo "[sql-optima] If the error mentions 'parent snapshot' / 'does not exist: not found', clear BuildKit cache:"
  echo "  docker builder prune -af"
  echo "  docker compose build --no-cache api"
  echo "  docker compose up -d"
  echo "[sql-optima] Otherwise (Vault): dev reset (deletes ALL compose volumes — TimescaleDB + Vault):"
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
echo "[sql-optima] First visit: setup wizard — create your admin username and password."
echo "[sql-optima] Then: add PostgreSQL or SQL Server (or use the in-app guide for local HA test clusters)."
echo ""
echo "Stop (keep data):  docker compose down"
echo "Stop + wipe data: docker compose down -v"
