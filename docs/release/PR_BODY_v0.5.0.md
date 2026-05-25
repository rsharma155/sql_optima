# Pull request: Release v0.5.0

**Title:** Release v0.5.0: Backup/DR dashboards, bash OS collector, collector & intelligence refinements, security hardening

**Base:** `main` ← **Head:** `cold_storage_refine_collector`

Open PR: https://github.com/rsharma155/sql_optima/compare/main...cold_storage_refine_collector?expand=1

---

## Summary

This PR ships **SQL Optima v0.5.0** — a major step toward production readiness for a self-hosted PostgreSQL and SQL Server observability platform. It adds **Backup & Recovery** dashboards for both engines, replaces the Go OS agent with a **bash host collector** (UI zip bundle + cron/systemd install), hardens **security defaults and XSS/API error handling**, and refines **background collectors**, **SQL Server HA/RPO-RTO monitoring**, and the **Intelligence Report** engine (v4 template, narrative enrichment, forecasting).

Built as a personal open-source project; treat this as a **beta / primitive first release** — feedback and contributions are welcome.

## Highlights

### New features

- **SQL Server Backup & Recovery** — posture from `msdb`, backup history trends, DR policy API, readiness UI (`frontend/js/pages/sqlserver_backups.js`)
- **PostgreSQL Backup & DR** — per-server DR policy (`GET/PUT /api/postgres/dr-policy`), readiness scoring, revamped `pg_backups` UI
- **OS collector (bash)** — `os_collector/sql-optima-os-collector.sh`; Admin/UI **Download bundle (.zip)** with pre-filled server ID/URL; ingest toggle in `optima_platform_settings`
- **Admin SQL Server diagnostics** — `GET /api/admin/diagnostics/sqlserver/{instance}` (Timescale row counts, collector health, no credential exposure)
- **OS-aware PostgreSQL rules** — host RAM merged into best-practice evaluation via `oscontext` + rule engine SQL
- **SQL Server query analysis** — statement fingerprint rollup, user-workload filters, extra Query V2 delta columns
- **Intelligence report** — narrative enrichment, **report v4** HTML template, in-memory report cache, capacity forecasting and wait analysis improvements
- **Cold storage** — `coldstorage.watermarks` / `coldstorage.runs` consolidated into `01_timescale_schema.sql`; admin UI for status/runs
- **CI/CD** — GHCR image publish on `v*.*.*` tags (`.github/workflows/release.yml`)

### Collector & HA refinements

- Refactored SQL Server/PostgreSQL background collectors and workers
- Robust **RPO/RTO** monitoring for SQL Server HA/AG
- New collectors: session, perf counters, index fragmentation, missing indexes
- Revamped DB init/permission scripts; cleanup of obsolete tables and deprecated logic
- Removed legacy `config.yaml` onboarding path (UI registry only)

### Security & operations

- Docker defaults: `AUTH_REQUIRED=1`, `DISABLE_PUBLIC_SETUP=1`, OS metrics ingest gated behind admin JWT
- XSS hardening (`escapeHtml` on server-derived HTML in dashboard pages)
- Sanitized API errors via `internal/apiresponse`
- Admin user CRUD audit events in `optima_audit_logs`
- New operator docs: `docs/operations.md`, `docs/vault_production.md`, `docs/release_engineering.md`

### Fixes

- PostgreSQL **kill session** and **reset queries** wired to real implementations (no longer stubbed)
- OS metrics ingest pipeline: flat agent payload → `monitor.pg_os_*` + `host_memory_samples`
- SQL Server workload/query-analysis empty-chart scenarios easier to diagnose via admin diagnostics
- PostgreSQL CPU/Memory dashboards: per-instance pgss user exclusion and memory drilldown improvements

## Breaking / upgrade notes

| Change | Action |
|--------|--------|
| Go `os_collector` removed | Deploy bash agent from UI zip or `os_collector/` |
| Query V2 always on | Remove `ENABLE_QUERY_V2_PIPELINE` env var |
| `07_cold_storage.sql` removed | Re-apply idempotent `01_timescale_schema.sql` |
| Stricter Docker auth | Set strong `JWT_SECRET`; create admin via `go run reset_password.go` |

Full notes: [CHANGELOG.md](../../CHANGELOG.md), [RELEASES.md](../../RELEASES.md).

## Test plan

- [ ] `cd backend && go test -race -timeout 120s ./...`
- [ ] `docker compose up --build` — UI at `http://localhost:8080`, register PG + SQL Server test instances
- [ ] SQL Server: open **Backup & Recovery**, **HA dashboard**, **Intelligence Report**, **Query Analysis** (fingerprint + workload filter)
- [ ] PostgreSQL: **Backup & DR**, **kill session** / **reset queries**, best practices with OS collector enabled
- [ ] Download OS collector zip from Admin → install on Linux PG host → verify Memory/CPU badges
- [ ] Admin → **Cold storage** status; `GET /api/admin/diagnostics/sqlserver/{instance}` as admin
- [ ] Upgrade path: re-apply `01_timescale_schema.sql` on existing TimescaleDB (idempotent)
- [ ] Tag `v0.5.0` and confirm GHCR publish workflow (if releasing)
