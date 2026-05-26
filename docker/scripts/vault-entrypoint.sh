#!/bin/sh
# Vault file-storage bootstrap for SQL Optima Docker Compose (dev / prod-lite).
set -eu

export VAULT_ADDR="${VAULT_ADDR:-http://127.0.0.1:8200}"

vault server -config=/vault/config/vault.hcl &
VAULT_PID=$!

# Wait for API (slow disks / first image pull on Windows Docker Desktop).
# Do NOT use "vault status" exit code here: uninitialized Vault returns non-zero
# even when the server is up (see "seal configuration missing, not initialized").
ready=0
i=0
while [ "$i" -lt 90 ]; do
  if vault status 2>&1 | grep -q 'Initialized'; then
    ready=1
    break
  fi
  i=$((i + 1))
  sleep 1
done
if [ "$ready" -ne 1 ]; then
  echo "[vault-init] ERROR: Vault API did not become ready within 90s"
  exit 1
fi

# First run: initialize and persist unseal key + root token on vault_data volume.
if ! vault status 2>/dev/null | grep -q 'Initialized.*true'; then
  echo "[vault-init] Initializing Vault for the first time..."
  vault operator init -key-shares=1 -key-threshold=1 -format=json > /vault/data/.init.json

  # Collapse JSON to one line (Vault may emit pretty-printed JSON).
  INIT_ONELINE=$(tr -d '\n\r\t ' < /vault/data/.init.json)
  UNSEAL_KEY=$(echo "$INIT_ONELINE" | sed -n 's/.*"unseal_keys_b64":\["\([^"]*\)".*/\1/p' | head -1)
  ROOT_TOKEN=$(echo "$INIT_ONELINE" | sed -n 's/.*"root_token":"\([^"]*\)".*/\1/p' | head -1)

  if [ -z "$UNSEAL_KEY" ] || [ -z "$ROOT_TOKEN" ]; then
    echo "[vault-init] ERROR: could not parse init output (see /vault/data/.init.json)"
    exit 1
  fi
  printf '%s\n' "$UNSEAL_KEY" > /vault/data/.unseal
  printf '%s\n' "$ROOT_TOKEN" > /vault/data/.root_token
  chmod 644 /vault/data/.root_token
  echo "[vault-init] Vault initialized. Root token stored to /vault/data/.root_token"
fi

# API container (nonroot) must read the token file from a shared volume mount.
if [ -f /vault/data/.root_token ]; then
  chmod 644 /vault/data/.root_token 2>/dev/null || true
fi

# Restart path: auto-unseal using the key on the persistent volume.
if vault status 2>/dev/null | grep -q 'Sealed.*true'; then
  if [ ! -s /vault/data/.unseal ]; then
    echo "[vault-init] ERROR: Vault is sealed but /vault/data/.unseal is missing"
    echo "[vault-init] Dev fix: docker compose down -v  (destroys Vault + DB volumes)"
    exit 1
  fi
  echo "[vault-init] Unsealing Vault..."
  if ! vault operator unseal "$(cat /vault/data/.unseal)"; then
    echo "[vault-init] ERROR: unseal failed - .unseal does not match vault_data"
    echo "[vault-init] Dev fix: docker compose down -v  (destroys Vault + DB volumes)"
    exit 1
  fi
fi

echo "[vault-init] Vault is ready (initialized and unsealed)"
wait "$VAULT_PID"
