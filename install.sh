#!/usr/bin/env bash
# SQL Optima — one-command install from anywhere (clone if needed, then start dev stack).
# Usage:
#   ./install.sh
#   ./install.sh --no-browser
#   SQL_OPTIMA_DIR=/path/to/repo ./install.sh
#   curl -fsSL https://raw.githubusercontent.com/rsharma155/sql_optima/main/install.sh | bash
set -euo pipefail

REPO_URL="${SQL_OPTIMA_REPO_URL:-https://github.com/rsharma155/sql_optima.git}"
DEFAULT_DIR="${SQL_OPTIMA_DIR:-}"
NO_BROWSER=0
NO_CLONE=0

usage() {
  cat <<'EOF'
SQL Optima — local dev install

Usage: install.sh [options]

Options:
  --dir PATH       Use or clone into PATH (default: ./sql_optima or SQL_OPTIMA_DIR)
  --no-browser     Do not open a browser when the API is ready
  --no-clone       Require current directory to be the repo root (no git clone)
  -h, --help       Show this help

Environment:
  SQL_OPTIMA_DIR       Target repo directory
  SQL_OPTIMA_REPO_URL  Git clone URL (default: GitHub sql_optima)
  SQL_OPTIMA_NO_BROWSER=1  Same as --no-browser
  SQL_OPTIMA_SETUP_MODE=easy|custom  Non-interactive setup (default: easy)
  SQL_OPTIMA_EXPOSE_DB_LAN=0|1       Publish TimescaleDB for LAN/DBeaver (custom only)
  SQL_OPTIMA_DB_* / SQL_OPTIMA_API_PORT  Optional credentials and ports (see docs/QUICKSTART.md)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      DEFAULT_DIR="${2:?--dir requires a path}"
      shift 2
      ;;
    --no-browser)
      NO_BROWSER=1
      shift
      ;;
    --no-clone)
      NO_CLONE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[sql-optima] Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ "${SQL_OPTIMA_NO_BROWSER:-}" == "1" ]]; then
  NO_BROWSER=1
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[sql-optima] ERROR: '$1' is not installed." >&2
    return 1
  fi
}

echo "[sql-optima] Checking prerequisites..."
require_cmd docker || {
  echo "[sql-optima] Install Docker Desktop: https://www.docker.com/products/docker-desktop/" >&2
  exit 1
}
if ! docker info >/dev/null 2>&1; then
  echo "[sql-optima] ERROR: Docker daemon is not running. Start Docker Desktop and retry." >&2
  exit 1
fi
if docker compose version >/dev/null 2>&1; then
  :
elif docker-compose version >/dev/null 2>&1; then
  echo "[sql-optima] WARNING: 'docker compose' (v2) is recommended; legacy docker-compose detected." >&2
else
  echo "[sql-optima] ERROR: Docker Compose v2 is required ('docker compose')." >&2
  exit 1
fi
require_cmd git || {
  echo "[sql-optima] ERROR: git is required to clone the repository." >&2
  exit 1
}

resolve_repo() {
  if [[ -f backend/go.mod ]]; then
    REPO_ROOT="$(pwd)"
    return 0
  fi
  if [[ -n "$DEFAULT_DIR" && -f "${DEFAULT_DIR}/backend/go.mod" ]]; then
    REPO_ROOT="$(cd "$DEFAULT_DIR" && pwd)"
    return 0
  fi
  local target="${DEFAULT_DIR:-./sql_optima}"
  if [[ -f "${target}/backend/go.mod" ]]; then
    REPO_ROOT="$(cd "$target" && pwd)"
    return 0
  fi
  if [[ "$NO_CLONE" -eq 1 ]]; then
    echo "[sql-optima] ERROR: Not inside the sql_optima repo and --no-clone was set." >&2
    exit 1
  fi
  echo "[sql-optima] Cloning ${REPO_URL} into ${target}..."
  git clone --depth 1 "$REPO_URL" "$target"
  REPO_ROOT="$(cd "$target" && pwd)"
}

resolve_repo
echo "[sql-optima] Using repository: ${REPO_ROOT}"

START_ARGS=()
[[ "$NO_BROWSER" -eq 1 ]] && START_ARGS+=(--no-browser)

exec "${REPO_ROOT}/docker/start-dev.sh" "${START_ARGS[@]}"
