# Releases

## Versioning
Until 1.0, releases may include breaking changes. We still aim to keep changes documented and predictable.

## Unreleased

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
