# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- **Helm starter chart** — `deploy/helm/sql-optima` for control-plane Deployment/Service; optional TimescaleDB subchart + schema Job (scripts 01–07).
- **Cold storage Phase 3** — `/api/config` cold-storage flags; global time picker 30d/90d presets; allowlisted `POST /api/cold-storage/history`; hot+cold merge for SQL Server CPU / memory / wait / connection history when Trino is configured.
- **Frontend federated history** — shared helper + source badges (hot / hot+cold) on SQL Server history charts.
- **OIDC group → role mapping** — `OIDC_GROUP_CLAIM` + `OIDC_GROUP_ROLE_MAP`.
- **RBAC role constants** — `middleware.RoleAdmin|RoleDBA|RoleViewer` + `NormalizeRole`.
- **Timescale retention helpers** — optional `014_timescale_retention_downsampling.sql` (90d floors + hourly CPU CAGG).
- **Alert notifications** — PagerDuty Events API v2 + native SMTP; `alert.resolved` notifications; scoped OS agent JWT + jti revoke list.
- **In-app Dashboard Info guides** — PostgreSQL (14) and SQL Server (16) metric/threshold reference pages.

### Changed
- Broader `apiresponse` sanitization across widget, storage-index, wait-stats, admin, SQL Server monitoring, query analysis, workload, and intelligence routes.
- Schema bootstrap step 7 is `07_optima_server_dr_policy.sql` (OS-enriched rules remain in `02_rule_engine.sql`).

## [0.5.0] - 2026-05-25

### Security
- Production Vault runbook ([`docs/vault_production.md`](docs/vault_production.md)); Docker Compose defaults hardened (`AUTH_REQUIRED=1`, `DISABLE_PUBLIC_SETUP=1`, OS metrics ingest gated behind admin JWT).
- XSS hardening across `frontend/js/pages/**` (`escapeHtml` on server-derived HTML).
- Sanitized API errors via `internal/apiresponse` on OS metrics ingest, cold storage, and admin user routes ([`docs/api_errors.md`](docs/api_errors.md)).
- Admin user create/update/delete events written to `optima_audit_logs`.

### Added
- **SQL Server Backup & Recovery** dashboard: collectors (`sqlserver_backup_posture`, `sqlserver_backup_history`), Timescale hypertables, APIs (`/api/sqlserver/backup/dashboard`, `/api/sqlserver/backup/policy`), and UI (`sqlserver_backups`).
- **PostgreSQL Backup & DR** improvements: per-server DR policy API (`GET/PUT /api/postgres/dr-policy`), readiness helpers, and revamped `pg_backups` UI.
- **OS collector (bash)**: Linux shell agent replaces the Go binary; UI **Download bundle (.zip)** with pre-filled instance/server ID/URL; ingest toggle in `optima_platform_settings` (no restart when enabled from UI).
- **Admin SQL Server diagnostics**: `GET /api/admin/diagnostics/sqlserver/{instance}` — Timescale row counts and collector health without exposing credentials ([`docs/operations.md`](docs/operations.md)).
- **OS-aware rule engine**: PostgreSQL RAM rules in `02_rule_engine.sql` + `internal/ruleengine/oscontext` merge host metrics into best-practice evaluation.
- **SQL Server query analysis**: statement fingerprint rollup, workload filter helpers, and additional Query V2 delta columns (`total_elapsed_ms`, `total_physical_reads`, `is_user_workload`).
- **Intelligence report**: narrative enrichment, report v4 template, and in-memory report cache for faster regeneration.
- **Cold storage schema** merged into `01_timescale_schema.sql` (`coldstorage.watermarks`, `coldstorage.runs` hypertable); standalone `07_cold_storage.sql` removed.
- Operator docs: [`docs/operations.md`](docs/operations.md), [`docs/os_collector.md`](docs/os_collector.md), [`docs/release_engineering.md`](docs/release_engineering.md).
- GHCR image publish on `v*.*.*` tags (`.github/workflows/release.yml`).

### Changed
- **Query V2 pipeline** is always enabled at API startup (no `ENABLE_QUERY_V2_PIPELINE` env var).
- **OS collector auth**: agent uses `Authorization: Bearer` (admin JWT) instead of `X-API-Key`.
- `postgres_stub_impl.go` renamed to `pg_stub_impl.go`.
- SQL Server **Memory Intelligence** collector cadence: 3600s → 60s (seed data).
- SQL Server historical dashboards require Timescale data by default; live DMV fallback is opt-in (`source=live`).
- Cold storage schema consolidated into `01_timescale_schema.sql` (no separate cold-storage migration step).

### Fixed
- PostgreSQL **kill session** and **reset queries** wired to real implementations (no longer stubbed).
- OS collector ingest pipeline: flat agent payload → `monitor.pg_os_*` + `host_memory_samples` bridge.
- SQL Server workload/query-analysis empty-chart scenarios easier to diagnose via admin diagnostics.

### Removed
- Go-based `os_collector` binary (`main.go`, `go.mod`) — use `sql-optima-os-collector.sh` only.
- Obsolete backend helper scripts (`update_pg_info.py`, `update_queries.py`, etc.) and committed `backend/main` binary.
- Standalone `infrastructure/sql_scripts/07_cold_storage.sql` (content in `01_timescale_schema.sql`).
- Standalone `infrastructure/sql_scripts/07_rule_engine_os_enriched.sql` (content in `02_rule_engine.sql`).

## [0.4.0] - 2026-05-19

See [RELEASES.md](RELEASES.md#040-2026-05-19).

[Unreleased]: https://github.com/rsharma155/sql_optima/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/rsharma155/sql_optima/releases/tag/v0.5.0
[0.4.0]: https://github.com/rsharma155/sql_optima/releases/tag/v0.4.0
