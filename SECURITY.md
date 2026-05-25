# Security Policy

## Supported versions

This repository is under active development. Security fixes are applied to the latest commit on the default branch and included in the next tagged release.

| Version | Supported |
|---------|-----------|
| 0.5.x   | Yes       |
| 0.4.x   | Best effort |
| < 0.4   | No        |

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.

- Email: create a private report to the maintainer (preferred), or open a draft PR marked **Security** with details redacted.
- Include: affected component, reproduction steps, impact assessment, and a suggested fix if you have one.

## Threat model

See [`docs/threat_model.md`](docs/threat_model.md) for assets, boundaries, and mitigations.

## Data Privacy & Telemetry

- **No Phone Home**: SQL Optima is completely self-hosted. It does **not** send any telemetry, usage statistics, or monitored database data to external servers.
- **Local Storage**: All collected metrics, alerts, and audit logs are stored in your local TimescaleDB instance.
- **Credential Safety**: Monitored server credentials never leave your infrastructure. They are encrypted at rest and only decrypted in-memory by the Go backend when establishing connections.

## Security principles (project rules)

- **Monitoring must be non-destructive**: dynamic SQL execution paths must remain **read-only**, **single-statement**, and **bounded** (row limit + timeout). Export handlers (`csv_export`, `pg_export`, `sqlserver_export`) apply the same constraints — they read but never write to monitored targets.
- **Least privilege**: monitored database users must have the minimum permissions needed (see `infrastructure/sql_scripts/pgsql_init.sql` and `infrastructure/sql_scripts/sqlserver_init.sql`). Prefer `dbmonitor_user` over `sa` / superuser in production.
- **No secrets in logs**: never log passwords, DSNs, access tokens, raw query text, or large result payloads by default. Use `internal/apiresponse` for client-facing errors on new/touched routes ([`docs/api_errors.md`](docs/api_errors.md)).
- **Safe defaults**: Docker Compose ships with `AUTH_REQUIRED=1` and `DISABLE_PUBLIC_SETUP=1`. Replace `JWT_SECRET` and database passwords before any shared deployment.
- **Credential encryption**: server credentials are encrypted at rest using Vault Transit KMS; falls back to local envelope encryption derived from `JWT_SECRET` when Vault is unavailable (dev/single-node only).
- **Auth-derived identity**: mutation endpoints (alerts, maintenance windows, admin user CRUD) extract actor identity from JWT claims — no client-supplied actor field is trusted. Admin user changes are appended to `optima_audit_logs`.
- **Singleton background jobs**: the alert evaluation loop uses `pg_try_advisory_xact_lock` to prevent duplicate evaluation across scaled API replicas.
- **XSS**: dashboard pages escape server-derived strings before inserting HTML (`escapeHtml` in page scripts).

## OS collector considerations

The bash OS agent authenticates with an **admin JWT** (`Authorization: Bearer`) to `POST /api/os/metrics`. Ingest is off until enabled from the UI or `OS_METRICS_INGEST_ENABLED=1`.

| Risk | Mitigation |
|------|------------|
| Compromised DB host exposes admin JWT | Store JWT only in root-owned `/etc/sql-optima/os-collector.env` (mode 600); rotate JWT periodically; limit host access. Future: scoped machine token (write-only OS metrics). |
| Unauthenticated metric spam | Ingest disabled by default; endpoint returns 403 when off; requires valid admin JWT when on. |
| Agent has no DB credentials | Agent only talks HTTPS to SQL Optima — it does not connect to PostgreSQL. |

See [`docs/os_collector.md`](docs/os_collector.md) and [`os_collector/README.md`](os_collector/README.md).

## Hardening checklist (operators)

- Run behind TLS (reverse proxy is fine).
- Restrict network access (VPN / private network / security groups).
- Use dedicated read-only database roles for monitoring.
- Store credentials in environment variables or a secrets manager.
- Configure Vault Transit for production credential encryption (`VAULT_ADDR`, AppRole — not root tokens). See [`docs/vault_production.md`](docs/vault_production.md).
- Set `AUTH_REQUIRED=1` and `DISABLE_PUBLIC_SETUP=1` after initial bootstrap.
- Set a strong `JWT_SECRET` (32+ random bytes); never use compose example values in production.
- Use `AUTH_MODE=oidc` with an external identity provider for enterprise SSO.
- Pin container images by digest or semver tag (`ghcr.io/<org>/sql-optima:0.5.0`), not `:latest`, in production.

## Vault (Transit KMS)

Monitored credentials are encrypted via Vault Transit. **Production operators** must follow [`docs/vault_production.md`](docs/vault_production.md): AppRole (not root tokens), TLS, HA cluster, and tested backups of `vault_data`.

### Quick recovery (Docker Compose dev)

- **Unseal key:** `/vault/data/.unseal` on the `vault_data` volume.
- **Root token (dev only):** `/vault/data/.root_token` — do not use in production.
- **Manual unseal:** `docker compose exec vault vault operator unseal $(cat /vault/data/.unseal)`
- **WARNING:** Deleting the `vault_data` volume (e.g. `docker compose down -v`) **permanently destroys** the ability to decrypt stored credentials.
