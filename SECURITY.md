# Security Policy

## Supported versions
This repository is under active development. Security fixes are applied to the latest commit on the default branch and included in the next tagged release.

## Reporting a vulnerability
Please **do not** open a public GitHub issue for security problems.

- Email: create a private report to the maintainer (preferred), or open a draft PR marked **Security** with details redacted.
- Include: affected component, reproduction steps, impact assessment, and a suggested fix if you have one.

## Threat model
See `docs/threat_model.md` for assets, boundaries, and mitigations.

## Security principles (project rules)
- **Monitoring must be non-destructive**: dynamic SQL execution paths must remain **read-only**, **single-statement**, and **bounded** (row limit + timeout).
- **Least privilege**: monitored database users must have the minimum permissions needed (see `infrastructure/security/roles/`).
- **No secrets in logs**: never log passwords, DSNs, access tokens, raw query text, or large result payloads by default.
- **Safe defaults**: when `AUTH_REQUIRED=1`, require a strong `JWT_SECRET`.
- **Credential encryption**: server credentials are encrypted at rest using Vault Transit KMS; falls back to local envelope encryption derived from `JWT_SECRET` when Vault is unavailable.
- **Auth-derived identity**: mutation endpoints (alert acknowledge/resolve, maintenance windows) extract actor identity from JWT claims — no client-supplied actor field is trusted.
- **Singleton background jobs**: the alert evaluation loop uses `pg_try_advisory_xact_lock` to prevent duplicate evaluation across scaled API replicas.

## Hardening checklist (operators)
- Run behind TLS (reverse proxy is fine).
- Restrict network access (VPN / private network / security groups).
- Use dedicated read-only database roles for monitoring.
- Store credentials in environment variables or a secrets manager.
- Configure Vault Transit for production credential encryption (`VAULT_ADDR`, `VAULT_TOKEN`, `VAULT_TRANSIT_KEY`).
- Set `AUTH_REQUIRED=1` and `DISABLE_PUBLIC_SETUP=1` after initial bootstrap.
- Use `AUTH_MODE=oidc` with an external identity provider for enterprise SSO.

