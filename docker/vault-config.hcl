# docker/vault-config.hcl
# Vault server configuration for SQL Optima Docker deployments.
# Uses the file storage backend so keys survive container restarts.

ui            = false
disable_mlock = true          # required in containers without IPC_LOCK privileges

storage "file" {
  path = "/vault/data"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = 1             # TLS is handled by the reverse proxy in production.
                              # For local dev this is acceptable; in production put
                              # Vault behind nginx/Caddy with a real TLS certificate.
}
