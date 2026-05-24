package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
)

// probeSQLServer opens a transient connection to verify credentials and network access.
// Uses the exact same DSN shape as NewSqlServerRepository so TLS/cert behaviour matches.
func probeSQLServer(ctx context.Context, host string, port int, user, pass, database string, trustCert bool) error {
	if port == 0 {
		port = 1433
	}
	catalog := strings.TrimSpace(database)
	if catalog == "" {
		catalog = "master"
	}

	msURL := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, fmt.Sprintf("%d", port)),
	}
	q := msURL.Query()
	q.Set("database", catalog)
	q.Set("encrypt", "true")
	q.Set("app name", "sql-optima-probe")
	if trustCert {
		q.Set("TrustServerCertificate", "true")
	} else {
		q.Set("TrustServerCertificate", "false")
	}
	msURL.RawQuery = q.Encode()

	db, err := sql.Open("sqlserver", msURL.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	return checkSQLServerPermissions(ctx, db)
}

func checkSQLServerPermissions(ctx context.Context, db *sql.DB) error {
	checks := []struct {
		name string
		sql  string
	}{
		{"VIEW SERVER STATE", "SELECT HAS_PERMS_BY_NAME(null, null, 'VIEW SERVER STATE')"},
		{"VIEW ANY DEFINITION", "SELECT HAS_PERMS_BY_NAME(null, null, 'VIEW ANY DEFINITION')"},
		{"VIEW ANY DATABASE", "SELECT HAS_PERMS_BY_NAME(null, null, 'VIEW ANY DATABASE')"},
	}

	var missing []string
	for _, c := range checks {
		var hasPerm int
		err := db.QueryRowContext(ctx, c.sql).Scan(&hasPerm)
		if err != nil || hasPerm != 1 {
			missing = append(missing, c.name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required permissions: %s", strings.Join(missing, ", "))
	}

	// Optional check for msdb access if we can
	var hasMsdb int
	_ = db.QueryRowContext(ctx, "SELECT CASE WHEN DB_ID('msdb') IS NOT NULL THEN 1 ELSE 0 END").Scan(&hasMsdb)
	if hasMsdb == 1 {
		// Just a smoke test for one table
		var canSelect int
		_ = db.QueryRowContext(ctx, "SELECT HAS_PERMS_BY_NAME('msdb.dbo.sysjobs', 'OBJECT', 'SELECT')").Scan(&canSelect)
		if canSelect != 1 {
			// We don't fail hard here as it might be optional, but we could log it.
			// However, for this task, we want to be thorough.
		}
	}

	return nil
}

// probePostgres opens a transient connection to verify credentials and network access.
func probePostgres(ctx context.Context, host string, port int, user, pass, database, sslMode string) error {
	if port == 0 {
		port = 5432
	}
	if strings.TrimSpace(database) == "" {
		database = "postgres"
	}
	if strings.TrimSpace(sslMode) == "" {
		sslMode = "disable"
	}

	pgURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		Path:   "/" + database,
	}
	q := pgURL.Query()
	q.Set("sslmode", sslMode)
	q.Set("connect_timeout", "10")
	pgURL.RawQuery = q.Encode()

	db, err := sql.Open("pgx", pgURL.String())
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	return checkPostgresPermissions(ctx, db)
}

func checkPostgresPermissions(ctx context.Context, db *sql.DB) error {
	// Check for pg_monitor or superuser
	var hasMonitor bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_roles 
			WHERE rolname = current_user 
			AND (rolsuper OR pg_has_role(current_user, 'pg_monitor', 'USAGE'))
		)`).Scan(&hasMonitor)

	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !hasMonitor {
		return fmt.Errorf("user lacks 'pg_monitor' role or superuser privileges")
	}

	// Check pg_stat_statements
	var hasStatStatements bool
	_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&hasStatStatements)
	if hasStatStatements {
		var canReadStats bool
		_ = db.QueryRowContext(ctx, "SELECT HAS_TABLE_PRIVILEGE(current_user, 'pg_stat_statements', 'SELECT')").Scan(&canReadStats)
		if !canReadStats {
			// Not a hard failure but good to know.
		}
	}

	return nil
}

// draftConnectTimeout is the per-probe deadline.
const draftConnectTimeout = 10 * time.Second
