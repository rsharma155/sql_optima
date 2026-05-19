package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

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
	return db.PingContext(ctx)
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
		sslMode = "prefer"
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
	return db.PingContext(ctx)
}

// draftConnectTimeout is the per-probe deadline.
const draftConnectTimeout = 10 * time.Second
