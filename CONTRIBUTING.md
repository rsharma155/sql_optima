# Contributing to SQL Optima

Thanks for helping improve SQL Optima. This project is a **database monitoring tool**—please treat security, correctness, and safety as first-class requirements.

## Development setup

### Prerequisites
- Go 1.25+
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

## Contribution guidelines

### Code quality bar
- **No destructive SQL** in monitoring execution paths. Dynamic SQL must remain **read-only**, **single-statement**, and **bounded** (row limit + timeout).
- **No secrets in logs**: never log passwords, DSNs, tokens, or raw query text/results by default.
- **Alert mutations** must derive actor identity from JWT claims — never trust a client-supplied actor field.
- Prefer small PRs with a clear scope and an obvious test plan.

### Database schema changes
- Add new goose migrations under `backend/migrations/` (format: `00NNN_description.sql`).
- Keep `infrastructure/sql_scripts/` in sync for Docker schema-setup (idempotent `IF NOT EXISTS`).

### Tests
- Add/extend tests for anything that touches:
  - query sandboxing (`backend/internal/security/sqlsandbox/`)
  - auth / routing / handlers (`backend/internal/api/`)
  - repository SQL logic (`backend/internal/repository/`)
  - alert engine domain, service, and handlers (`backend/internal/domain/alerts/`, `backend/internal/service/`, `backend/internal/api/handlers/`)

### Commit / PR expectations
- Describe the **why** and include a short **test plan** (commands run + what you verified).
- Note any operational impact (new env vars, migrations, config changes).

## Reporting security issues
Please do **not** open public issues for security findings. See `SECURITY.md`.

