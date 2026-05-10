package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
)

type Driver string

const (
	DriverSQLServer Driver = "sqlserver"
	DriverPostgres  Driver = "postgres"
)

type Config struct {
	Driver           Driver
	ConnectionString string
	QueryTimeout     time.Duration
}

type Reader struct {
	config Config
	db     *sql.DB
}

func NewReader(cfg Config) (*Reader, error) {
	var driverName string
	switch cfg.Driver {
	case DriverSQLServer, "":
		driverName = "mssql"
	case DriverPostgres:
		driverName = "pgx"
	default:
		return nil, fmt.Errorf("unsupported database driver: %q (use %q or %q)", cfg.Driver, DriverSQLServer, DriverPostgres)
	}

	db, err := sql.Open(driverName, cfg.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	db.SetMaxOpenConns(1)

	return &Reader{config: cfg, db: db}, nil
}

func (r *Reader) Close() error {
	return r.db.Close()
}

func (r *Reader) Ping(ctx context.Context) error {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()
	return r.db.PingContext(ctx)
}

func (r *Reader) FetchPlanXML(ctx context.Context, query string) ([]byte, error) {
	ctx, cancel := r.withTimeout(ctx)
	defer cancel()

	var xmlData []byte
	err := r.db.QueryRowContext(ctx, query).Scan(&xmlData)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plan XML: %w", err)
	}

	if len(xmlData) == 0 {
		return nil, fmt.Errorf("query returned empty result")
	}

	return xmlData, nil
}

func (r *Reader) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := 30 * time.Second
	if r.config.QueryTimeout > 0 {
		timeout = r.config.QueryTimeout
	}
	return context.WithTimeout(ctx, timeout)
}
