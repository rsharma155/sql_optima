# Roadmap

This roadmap is intentionally practical: it focuses on adoption blockers (safety, deployment, trust signals) and operator value.

## Completed (0.x)
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
- **Security hardening**
  - Expand query sandbox test coverage (edge cases, dialect oddities).
  - Reduce error leakage across handlers (consistent redaction + request correlation).
- **Structured logging**
  - Replace `log.Printf` with a structured logging package carrying request ID, route, and duration.
- **Packaging**
  - Publish versioned images to GHCR.
- **Alert engine extensions**
  - Additional evaluators (long-running queries, connection pool saturation, WAL growth).
  - Alert routing integrations (Slack, PagerDuty, email webhooks).
- **OS collector wiring**
  - Complete host-level CPU/memory agent → dashboard integration (agent push → handler → chart pipeline).

## Medium-term (1.0 readiness)
- **Production model**
  - Clear “control plane vs agent” separation (remote collectors, mTLS).
  - Storage retention policies and downsampling strategies for TimescaleDB.
- **RBAC refinement**
  - Predefined role constants (viewer, dba, admin) with granular endpoint mapping.
  - Audit logging for all admin mutations (widget SQL edits, user operations, server registration).
- **Release engineering**
  - Versioned releases + changelog automation.
  - Signed artifacts (optional) and SBOM generation.

## Longer-term
- Kubernetes Helm chart for multi-environment deployments.
- Multi-tenant mode (namespacing instances, separate storage).
- OIDC group-to-role mapping for enterprise SSO.

