# Roadmap

This roadmap is intentionally practical: it focuses on adoption blockers (safety, deployment, trust signals) and operator value.

## Completed (0.x)
- **Alert engine extensions** — evaluators for SQL Server long-running queries / connection pressure (% when `max_connections` collected, else absolute), PostgreSQL connection saturation / WAL slot retention / PgBouncer wait; PagerDuty Events API v2 + native SMTP email; `alert.resolved` notifications; scoped OS agent JWT + jti revoke list for metric ingest.
- **API error sanitization** — widgets, storage-index, wait-stats, admin notification/collector, SQL Server monitoring, query analysis, workload, intelligence, and admin server paths use `apiresponse` (no internal `err.Error()` on 500s).
- **Cold storage Phase 3** — `/api/config` exposes enablement; global time picker 30d/90d presets; allowlisted `POST /api/cold-storage/history`; hot+cold merge for SQL Server CPU / memory / wait / connection history when Trino is configured.
- **Frontend federated history** — dashboards/drilldowns call `cpu|memory|wait|connection-history` via shared helper; source badges for hot / hot+cold; Health V2 overlays wait/connection series.
- **RBAC role constants** — `middleware.RoleAdmin|RoleDBA|RoleViewer` + `NormalizeRole`; widget SQL mutations audited.
- **OIDC group → role mapping** — `OIDC_GROUP_CLAIM` + `OIDC_GROUP_ROLE_MAP` for enterprise SSO.
- **Release engineering** — GHCR publish on `v*.*.*` with SPDX SBOM + GitHub Release notes from CHANGELOG.
- **Helm starter chart** — `deploy/helm/sql-optima` for control-plane Deployment/Service.
- **Timescale retention helpers** — optional `014_timescale_retention_downsampling.sql` (90d floors + hourly CPU CAGG).
- **Docker Compose** — one-command deploy (`docker/docker-compose.yml`) with TimescaleDB, Vault, and automatic schema bootstrap.
- **Prometheus metrics** — `/metrics` endpoint with request counters and duration histograms (`sql_optima_http_*`).
- **OpenTelemetry tracing** — optional OTLP HTTP exporter.
- **CI** — GitHub Actions: `go vet`, `go test -race`, coverage, `golangci-lint`, `gosec` SAST, Trivy dependency scan.
- **Alert engine** — cross-engine evaluators (7 built-in rules), fingerprint-based dedup, maintenance windows, audit history, advisory-lock singleton execution.
- **Vault Transit KMS** — credential encryption at rest with local envelope fallback.
- **RBAC middleware** — JWT auth with role-gated endpoints (`RequireAuth`, `RequireAnyRole`).
- **EXPLAIN analyzer** — PostgreSQL plan parser, diagnostics, metrics, index advisor, and report generator.
- **Platform compose** — production profile with Redis/Asynq worker, Prometheus, and Grafana (`docker-compose.platform.yml`).
- **Query V2 Pipeline** — hash-based delta tracking for SQL Server and PostgreSQL query metrics; per-query trend analysis enabled by default.
- **SQL Server Locks & Blocking Dashboard** — full-stack locks monitoring with drilldown detail views.
- **Data Export (CSV)** — CSV export for both SQL Server and PostgreSQL query and metric data.
- **PostgreSQL Incident Feed** — connectivity and incident tracking dashboard; connection utilization handler.
- **Collector engine refactor** — domain-driven architecture with isolated, single-responsibility collectors; modularized `pg_stats`.
- **Rule engine expansion** — 15+ new signal-aware best-practice evaluators for both engines.
- **SQL Server Dashboard redesign** — real-time triage view with 4-column KPI layout and per-section time-range selection.

## Near-term (0.x)
- Expand Helm chart with optional TimescaleDB subchart / schema Job.

## Medium-term (1.0 readiness)
- **Production model**
  - Clear “control plane vs agent” separation (remote collectors, mTLS).
  - Broader continuous-aggregate coverage beyond CPU.
- **RBAC refinement**
  - Granular endpoint capability matrix beyond role constants.
- **Release engineering**
  - Cosign/sigstore image signing (optional).

## Longer-term
- Multi-tenant mode (namespacing instances, separate storage).
- Full remote collector mesh with mTLS.
