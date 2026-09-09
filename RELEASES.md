# Releases

## Versioning

Until 1.0, releases may include breaking changes. We still aim to keep changes documented and predictable.

**Source of truth:** [`VERSION`](VERSION) at repo root. Tag releases as `v0.5.0` (with `v` prefix) to trigger GHCR publish.

## 0.5.0 (2026-05-25)

### Security & operations (v1.0 gate)
- **Vault production guide** ([`docs/vault_production.md`](docs/vault_production.md)), hardened Docker defaults (`AUTH_REQUIRED=1`, public setup lockdown, OS metrics behind admin JWT).
- **XSS sweep** on dashboard pages; **sanitized API errors** via `internal/apiresponse` on selected admin/ingest routes.
- **Operations guide** ([`docs/operations.md`](docs/operations.md)): in-memory metric delta reset after API restart (P2-7), SQL Server collector diagnostics endpoint.
- **Admin audit**: user CRUD events in `optima_audit_logs`.
- **Release engineering**: [`CHANGELOG.md`](CHANGELOG.md), GHCR publish on `v*.*.*` tags ([`.github/workflows/release.yml`](.github/workflows/release.yml)).

### New features
- **SQL Server Backup & Recovery**: Per-database posture from `msdb`, backup history trends, RPO policy (`optima_server_dr_policy`), readiness chips, and UI dashboard. Collectors: `sqlserver_backup_posture` (5 min), `sqlserver_backup_history` (15 min).
- **PostgreSQL Backup & DR**: DR policy API, readiness scoring, and refreshed backup UI (`pg_backups`).
- **OS collector (bash)**: Replaces Go agent. UI zip bundle with pre-filled server ID; **Enable ingest** toggle stored in `optima_platform_settings` (no API restart). See [`docs/os_collector.md`](docs/os_collector.md) and [`os_collector/README.md`](os_collector/README.md).
- **Admin SQL Server diagnostics**: `GET /api/admin/diagnostics/sqlserver/{instance}` — row counts per Timescale table, collector hints (admin role).
- **OS-aware PostgreSQL rules**: Host-RAM-aware best-practice evaluation via `oscontext` (rules live in `02_rule_engine.sql`; standalone `07_rule_engine_os_enriched.sql` was removed).
- **SQL Server query analysis**: Statement fingerprint grouping, user-workload filter, extra Query V2 delta columns.
- **Intelligence report**: Narrative enrichment, v4 HTML template, report cache.

### Schema & collectors
- Cold storage control tables merged into `01_timescale_schema.sql`; `07_cold_storage.sql` removed.
- New hypertables: `monitor.sqlserver_backup_database_posture`, `monitor.sqlserver_backup_history`.
- Retention policies added for several SQL Server hypertables; `optima_platform_settings` for UI-managed toggles.
- Setup wizard step 7: `07_optima_server_dr_policy.sql` (DR / RPO policy table).

### Breaking / upgrade notes
- **OS collector**: Remove any deployed Go `os_collector` binary; install the bash agent from the UI bundle or [`os_collector/sql-optima-os-collector.sh`](os_collector/sql-optima-os-collector.sh).
- **Existing TimescaleDB**: Re-apply `01_timescale_schema.sql` (idempotent `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS`) or run pending files under `infrastructure/sql_scripts/migrations/`. If you previously ran `07_cold_storage.sql`, the objects now live in `01` — no duplicate run needed.
- **Docker `.env`**: New installs default `AUTH_REQUIRED=1` and `DISABLE_PUBLIC_SETUP=1`. Set a strong `JWT_SECRET` before first use.
- **Query V2**: No env toggle; pipeline is always on.

### Removed
- Go `os_collector` (`main.go`, committed binary).
- Standalone `07_cold_storage.sql`.

## 0.4.0 (2026-05-19)

### New features
- **Query V2 Pipeline**: Hash-based delta tracking for SQL Server and PostgreSQL query metrics; computes per-query deltas between collection cycles for trend analysis. Always enabled (not configurable).
- **SQL Server Locks & Blocking Dashboard**: Full-stack locks monitoring — new API endpoints, `sqlserver_blocking_logger`, and UI pages (`sqlserver_LocksDashboard.js`, `sqlserver_LocksDrilldown.js`, `sqlserver_LocksDrilldownDetailed.js`).
- **PostgreSQL Incident Feed & Connectivity**: New `pg_incident_feed` handler and `pg_connection_utilization_handler` for advanced Postgres observability; incident feed route (`pg-incident`).
- **Data Export (CSV)**: CSV export handlers for both engines (`csv_export.go`, `pg_export.go`, `sqlserver_export.go`).
- **SQL Server Dashboard redesign**: Optimized real-time triage view with 4-column KPI layout, per-section time-range selection capped at 7 days, and live delta metrics.
- **New SQL Server routes**: `sqlserver-health-v2`, `sqlserver-waits`, `sqlserver-workload`, `sqlserver-locks`, `sqlserver-locks-drilldown`, `query-analysis`, `watched-queries`, `sqlserver-plan-analyzer`.
- **New PostgreSQL routes**: `pg-waits`, `pg-backup-dr`, `pg-security`, `pg-stat-statements`.

### Refactors
- **SQL Server Intelligence Report**: Fully ported the Python-based intelligence engine to Go; removed the standalone `intelligence-report` service from Docker Compose and integrated the autonomous health analysis directly into the Go backend.
- **Collector Engine**: Unified internal collectors into a domain-driven architecture (`application/`, `domain/`, `infrastructure/sqlserver/`, `infrastructure/timescaledb/`, `postgres/`).
- **PostgreSQL collector modularization**: Split `pg_stats` into focused single-responsibility collectors (`pg_comprehensive_collector.go`, `pg_snapshot_collector.go`, `pg_locks_blocking/`, etc.) to isolate failures and improve resilience.
- **Rule Engine Expansion**: 15+ new best-practice rule evaluators for both PG and SQL Server; rules are now signal-aware and evaluate against historical TimescaleDB snapshots.
- **Storage/Hot Layer**: Extensive updates across the TimescaleDB storage logger layer for new intelligence metrics (memory, blocking, deadlocks).

### Build
- Updated Go runtime to **1.26.1** (`go.mod`).
- Updated Docker build image to `golang:1.26`.
- Reordered Dockerfile `COPY` commands to support local module replacement.

## 0.3.0 (2026-04-22)

- **SQL Server Micro-Architecture**: Massive refactor of the SQL Server repository. Split the monolithic `mssql_stats.go` into 14 domain-specific files (e.g., `sqlserver_query_store.go`, `sqlserver_ag_health.go`) following DDD principles.
- **Engine Standardization**: Renamed all `mssql` references to `sqlserver` across the backend and frontend for consistency.
- **Push-based OS Collector**: Introduced `os_collector`, a lightweight Go agent for host-level telemetry (CPU, Memory, Load, Postgres process stats) that pushes data to the backend via `POST /api/os/metrics`.
- **Enhanced Rules Engine (V2)**: Refactored the advisory system to be "signal-aware," allowing rules to evaluate against historical snapshots in TimescaleDB using the `expr` language. Added 15+ new best-practice rules for both PG and SQL Server.
- **Storage & Index Health**: Fully integrated historical dashboards surfacing index usage deltas, unused index candidates, and table growth projections.
- **Tech Stack Updates**: Updated to Go 1.25.7 (bumped to 1.26.1 in 0.4.0); added `github.com/expr-lang/expr` for dynamic rule evaluation and `github.com/shirou/gopsutil` for host metrics.

## 0.2.1

- **Bug fix — Alert Ack/Resolve 400 error**: `AcknowledgeAlert` and `ResolveAlert` handlers now treat the request body as optional; an empty POST body no longer returns `400 Bad Request`.
- **Bug fix — "Invalid Date" in charts**: Chart time-series labels are now RFC 3339 ISO timestamps.
- **Bug fix — `toLocalTime` ReferenceError in replication.js**.
- **Bug fix — Autovacuum & Bloat Risk UI mismatch**.
- **Observability — Query Performance silent failure**: Now logs when `pg_stat_statements` is missing.
- **Admin — Permission check endpoints**: `POST /api/admin/servers/check-permissions-draft` and `POST /api/admin/servers/{id}/check-permissions`.

## 0.2.0

- **Alert engine** (Epic 1.1): cross-engine alert evaluation with fingerprint-based deduplication, maintenance windows, audit history, and 7 built-in evaluators.
- Alert evaluation loop uses `pg_try_advisory_xact_lock` for singleton execution in multi-replica deployments.
- Schema: new `optima_alerts`, `optima_alert_history`, `optima_maintenance_windows`, `optima_alert_rules` tables.
- Security: enforce read-only + row-limited execution for widget/rule/live helpers.

## 0.1.0 (initial)

- Go backend API + static SPA UI
- SQL Server + PostgreSQL monitoring dashboards
- Optional TimescaleDB-backed historical metrics
