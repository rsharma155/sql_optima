# Releases

## Versioning
Until 1.0, releases may include breaking changes. We still aim to keep changes documented and predictable.

## Unreleased

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
