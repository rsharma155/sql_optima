# Contributing to SQL Optima

Thanks for helping improve SQL Optima. This project is a **database monitoring tool**—please treat security, correctness, and safety as first-class requirements.

## Development setup

### Prerequisites
- Go 1.26+
- Docker (recommended for TimescaleDB / local infra)

### Run the backend (local)
- From repo root:

```bash
cd backend
go test ./...
go run ./cmd/server
```

### Run with Docker Compose
- From repo root:

```bash
cd docker
docker compose up --build
```

One-command local stack: `./install.sh` (macOS/Linux) or `.\install.ps1` (Windows). See [docs/QUICKSTART.md](docs/QUICKSTART.md).

### Optional: Helm
- Starter chart: [`deploy/helm/sql-optima/README.md`](deploy/helm/sql-optima/README.md).

## Contribution guidelines

### Code quality bar
- **No destructive SQL** in monitoring execution paths. Dynamic SQL must remain **read-only**, **single-statement**, and **bounded** (row limit + timeout).
- **No secrets in logs**: never log passwords, DSNs, tokens, or raw query text/results by default.
- **Alert mutations** must derive actor identity from JWT claims — never trust a client-supplied actor field.
- Prefer small PRs with a clear scope and an obvious test plan.

### Database schema changes
- Prefer idempotent updates in `infrastructure/sql_scripts/` (numbered **01–07** for first-run bootstrap; optional files under `migrations/` for later opt-in changes).
- Goose migrations under `backend/migrations/` exist for historical/alternate paths — keep Docker/Helm schema-setup scripts in sync when changing schema.
- All first-run scripts must use `IF NOT EXISTS` / equivalent idempotent DDL.

### Tests
- Add/extend tests for anything that touches:
  - query sandboxing (`backend/internal/security/sqlsandbox/`)
  - auth / routing / handlers (`backend/internal/api/`)
  - repository SQL logic (`backend/internal/repository/`)
  - alert engine domain, service, and handlers (`backend/internal/domain/alerts/`, `backend/internal/service/`, `backend/internal/api/handlers/`)
  - cold storage / federation (`backend/internal/storage/cold/`)

### Commit / PR expectations
- Describe the **why** and include a short **test plan** (commands run + what you verified).
- Note any operational impact (new env vars, migrations, config changes).
- Follow product naming: monitored engine is **SQL Server** (not MSSQL).

## Reporting security issues
Please do **not** open public issues for security findings. See `SECURITY.md`.
