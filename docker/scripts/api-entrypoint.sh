#!/bin/sh
# Inject Vault root token before starting the API (works with distroless binary via env).
set -eu

TOKEN_FILE="${VAULT_TOKEN_FILE:-/vault/token/.root_token}"

if [ -z "${VAULT_TOKEN:-}" ] || [ "${VAULT_TOKEN}" = "root" ]; then
  if [ -r "$TOKEN_FILE" ]; then
    VAULT_TOKEN=$(tr -d '\n\r' < "$TOKEN_FILE")
    export VAULT_TOKEN
    echo "[api] Using Vault token from $TOKEN_FILE"
  else
    echo "[api] WARNING: Vault token file not readable at $TOKEN_FILE (check vault_data mount and permissions)"
  fi
fi

exec /srv/backend/sql-optima "$@"
