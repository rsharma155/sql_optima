# Database Migrations (Goose)

All **application schema** changes are managed via [goose](https://github.com/pressly/goose) migration files in this directory.

Time-series / metric hypertables remain in `infrastructure/sql_scripts/` and are provisioned separately (setup API or manual import).

## Quick start

```bash
# Option A – use the built-in migrate CLI (no external install):
export GOOSE_DBSTRING="postgres://user:pass@localhost:5432/dbname?sslmode=disable"
go run ./cmd/migrate -dir migrations status   # show applied / pending
go run ./cmd/migrate -dir migrations up        # apply all pending migrations

# Option B – use the standalone goose binary:
go install github.com/pressly/goose/v3/cmd/goose@latest
goose -dir migrations postgres "$GOOSE_DBSTRING" status
goose -dir migrations postgres "$GOOSE_DBSTRING" up
```

## Creating a new migration

```bash
go run ./cmd/migrate -dir migrations create add_some_feature sql
# → creates migrations/NNNNN_add_some_feature.sql with Up / Down stubs
```

Edit the generated file and add your DDL inside `-- +goose Up` / `-- +goose Down` blocks.

## Existing databases

Migration `00002_core_application_tables.sql` is the baseline. It uses `CREATE TABLE IF NOT EXISTS` so it is safe to run against databases that were originally provisioned with `infrastructure/sql_scripts/00_timescale_schema.sql`.

## Rolling back

```bash
go run ./cmd/migrate -dir migrations down   # undo the last applied migration
```
