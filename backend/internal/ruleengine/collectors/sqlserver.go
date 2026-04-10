package collectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
	"github.com/rsharma155/sql_optima/internal/security/sqlsandbox"
)

type SQLServerCollector struct {
	connStr string
	db      *sql.DB
}

func NewSQLServerCollector(connStr string) (*SQLServerCollector, error) {
	db, err := sql.Open("mssql", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open mssql connection: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(3)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping mssql: %w", err)
	}

	return &SQLServerCollector{connStr: connStr, db: db}, nil
}

func (c *SQLServerCollector) Close() {
	if c.db != nil {
		c.db.Close()
	}
}

func (c *SQLServerCollector) ExecuteQuery(ctx context.Context, query string, timeout time.Duration) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	return c.scanRows(rows)
}

func (c *SQLServerCollector) scanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	columnCount := len(columns)
	values := make([]interface{}, columnCount)
	valuePtrs := make([]interface{}, columnCount)
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	var results []map[string]interface{}

	for rows.Next() {
		err := rows.Scan(valuePtrs...)
		if err != nil {
			log.Printf("[SQLServerCollector] Scan error: %v", err)
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = c.normalizeValue(values[i], col)
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

func (c *SQLServerCollector) normalizeValue(val interface{}, colName string) interface{} {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case []byte:
		if len(v) == 0 {
			return nil
		}
		str := string(v)
		if strings.HasSuffix(strings.ToLower(colName), "_desc") || strings.HasSuffix(strings.ToLower(colName), "_name") {
			return str
		}
		if str == "ON" || str == "YES" || str == "TRUE" {
			return true
		}
		if str == "OFF" || str == "NO" || str == "FALSE" {
			return false
		}
		return str

	case sql.NullString:
		if !v.Valid {
			return nil
		}
		return v.String

	case sql.NullInt64:
		if !v.Valid {
			return nil
		}
		return v.Int64

	case sql.NullFloat64:
		if !v.Valid {
			return nil
		}
		return v.Float64

	case sql.NullBool:
		if !v.Valid {
			return nil
		}
		return v.Bool

	default:
		return v
	}
}

func (c *SQLServerCollector) ExecuteRule(ctx context.Context, detectionSQL string) ([]map[string]interface{}, string, error) {
	wrapped, err := sqlsandbox.WrapWithRowLimit("sqlserver", detectionSQL, sqlsandbox.DefaultMaxRows)
	if err != nil {
		return nil, "", fmt.Errorf("sql sandbox: %w", err)
	}
	results, err := c.ExecuteQuery(ctx, wrapped, 30*time.Second)
	if err != nil {
		return nil, "", fmt.Errorf("detection query failed: %w", err)
	}

	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal results: %w", err)
	}

	return results, string(resultsJSON), nil
}

func (c *SQLServerCollector) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.db.PingContext(ctx)
}
