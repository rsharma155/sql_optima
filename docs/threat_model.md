# SQL Optima — threat model (summary)

This document maps trust boundaries and controls for the monitoring stack. It complements deployment-specific risk assessments.

## Assets

| Asset | Sensitivity |
|-------|-------------|
| Target DB credentials (monitored instances) | Critical — full DB access if compromised |
| App user passwords (`optima_users`) | High — dashboard and API access |
| JWT signing key / OIDC trust | High — session forgery |
| Metrics and DMV snapshots in TimescaleDB | High — query text, schema names, load patterns |
| Rule engine `detection_sql` in app DB | High — arbitrary read SQL if editor compromised |

## Trust boundaries

1. **Browser ↔ API** — HTTPS assumed in production; JWT Bearer or OIDC-validated tokens when `AUTH_REQUIRED` is enabled.
2. **API ↔ monitored databases** — TLS to SQL Server/PostgreSQL where configured; least-privilege logins (`infrastructure/security/roles/`).
3. **API ↔ app Postgres / Timescale** — connection strings via env; widget SQL runs only against configured pools after sandbox checks.
4. **Future: agent ↔ control plane** — not in scope for this revision; use mTLS and signed payloads when introduced.

## Attack surfaces and mitigations

| Surface | Risk | Mitigations |
|---------|------|-------------|
| REST API exposing instance metadata | Unauthorized disclosure | `AUTH_REQUIRED=1`, RBAC (`viewer` / `dba` / `admin`), audit logs on denials and mutations |
| `/api/login` | Brute force | Rate limiting (`LOGIN_RATE_LIMIT_PER_MIN`) |
| Ad-hoc / widget / rule SQL | Data exfiltration, DoS | SQL sandbox (read-only, timeouts, row limits), read-only DB roles |
| Stored credentials in config | Theft from disk | Env-only passwords in production; envelope encryption for future connection registry |
| Default JWT secret | Token forgery | `JWT_SECRET` required when `AUTH_REQUIRED=1` |

## Route posture

- **Always public (typical):** `GET /api/health`, `GET /api/health/ready`, `GET /api/auth/status`, `POST /api/login`, `POST /api/auth/login` (when user repo configured).
- **Public when `AUTH_REQUIRED` unset/false:** Full dashboard read API (development / legacy).
- **Authenticated when `AUTH_REQUIRED=true`:** All monitoring read routes require JWT with role `viewer`, `dba`, or `admin`. Admin routes require `admin`. See `RegisterHealthRoutes` in `internal/api/router.go`.

## OIDC

When `AUTH_MODE=oidc`, the API validates access tokens from the configured issuer (`OIDC_ISSUER_URL`, `OIDC_AUDIENCE`). Local username/password login remains available for `AUTH_MODE=local` (default).

## Review cadence

Revisit after major features (agents, multi-tenant SaaS, stored connection registry).
