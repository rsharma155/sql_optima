# Database migrations (Goose)

Versioned schema changes for the application and TimescaleDB should live here as SQL files.

Example:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
export GOOSE_DRIVER=postgres
export GOOSE_DBSTRING="postgres://user:pass@localhost:5432/dbname?sslmode=disable"
goose -dir migrations postgres status
goose -dir migrations postgres up
```

Initial deployments may continue to use `infrastructure/sql_scripts` until migrations are fully ported.
