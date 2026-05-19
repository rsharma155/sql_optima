# Releases

## Versioning
Until 1.0, releases may include breaking changes. We still aim to keep changes documented and predictable.

## Unreleased

No unreleased changes.

## 0.4.0 (2026-05-19)

### New features
- **Query V2 Pipeline**: Hash-based delta tracking for SQL Server and PostgreSQL query metrics; computes per-query deltas between collection cycles for trend analysis. Enabled by default (`ENABLE_QUERY_V2_PIPELINE=true`).
- **SQL Server Locks & Blocking Dashboard**: Full-stack locks monitoring — new API endpoints, `sqlserver_blocking_logger`, and UI pages (`sqlserver_LocksDashboard.js`, `sqlserver_LocksDrilldown.js`, `sqlserver_LocksDrilldownDetailed.js`).
- **PostgreSQL Incident Feed & Connectivity**: New `pg_incident_feed` handler and `pg_connection_utilization_handler` for advanced Postgres observability; incident feed route (`pg-incident`).
- **Data Export (CSV)**: CSV export handlers for both engines (`csv_export.go`, `pg_export.go`, `sqlserver_export.go`).
- **SQL Server Dashboard redesign**: Optimized real-time triage view with 4-column KPI layout, per-section time-range selection capped at 7 days, and live delta metrics.
- **New SQL Server routes**: `sqlserver-health-v2`, `sqlserver-waits`, `sqlserver-workload`, `sqlserver-locks`, `sqlserver-locks-drilldown`, `query-analysis`, `watched-queries`, `sqlserver-plan-analyzer`.
- **New PostgreSQL routes**: `pg-waits`, `pg-backup-dr`, `pg-security`, `pg-stat-statements`.

### Refactors
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
- **Bug fix — "Invalid Date" in charts**: Chart time-series labels were emitted as `"HH:MM"` strings (e.g. `"14:32"`), which `new Date()` cannot parse. Labels are now RFC 3339 ISO timestamps; the existing `toLocalTime` helper formats them as `HH:MM` in the browser.
- **Bug fix — `toLocalTime` ReferenceError in replication.js**: `toLocalTime` was scoped inside the `if (replCtx)` block but used in the sibling `if (checkCtx)` block. Moved to the enclosing scope and replaced the `try/catch` guard with an `isNaN` check so "Invalid Date" strings are never surfaced.
- **Bug fix — Autovacuum & Bloat Risk UI mismatch**: Replaced Bootstrap-style `card dashboard-card` / `table table-sm table-hover` classes with the project design system (`table-card glass-panel` / `data-table`) and updated the page title to the standard `<h1>` + `<p class="subtitle">` pattern.
- **Observability — Query Performance silent failure**: `collectPostgresQueryStatsSnapshotForInstance` previously swallowed errors (including `pg_stat_statements` not installed) without logging. Now emits a `log.Printf` for both the error path and the empty-result path so operators can diagnose why query captures are missing.
- **Admin — Permission check endpoints**: Added `POST /api/admin/servers/check-permissions-draft` and `POST /api/admin/servers/{id}/check-permissions` to probe monitoring role permissions and return ready-to-run `GRANT` and `CREATE USER` SQL scripts.

## 0.2.0
- **Alert engine** (Epic 1.1): cross-engine alert evaluation with fingerprint-based deduplication, maintenance windows, audit history, and 7 built-in evaluators (SQL Server: blocking, failed jobs, disk space; PostgreSQL: replication lag, blocking, backup freshness, disk space).
- Alert evaluation loop uses `pg_try_advisory_xact_lock` for singleton execution in multi-replica deployments.
- Alert mutation endpoints derive actor identity from JWT claims (no client-supplied actor field).
- Alert HTTP handlers return proper status codes: 400 (invalid ID / malformed body), 404 (not found), 409 (already resolved).
- Schema: new `optima_alerts`, `optima_alert_history`, `optima_maintenance_windows`, `optima_alert_rules` tables with partial unique index for dedup.
- Docker: `schema-setup` now applies `04_alert_engine.sql` automatically.
- Security: enforce read-only + row-limited execution for widget/rule/live helpers; reduce sensitive logging.
- Docs: add OSS trust files (this document, contributing, security, architecture, roadmap, conduct).

## 0.1.0 (initial)
- Go backend API + static SPA UI
- SQL Server + PostgreSQL monitoring dashboards
- Optional TimescaleDB-backed historical metrics
