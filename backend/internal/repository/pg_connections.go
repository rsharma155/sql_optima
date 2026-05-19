// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL connection pool metrics including active, idle, and total connection counts.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
)

// CollectPgConnections fetches PostgreSQL connection stats
func (c *PgRepository) CollectPgConnections(ctx context.Context, db *sql.DB) (int, int, int, error) {
	query := `
		SELECT /* SQL_OPTIMA */   
			COALESCE(SUM(CASE WHEN state = 'active' THEN 1 ELSE 0 END), 0) AS active_connections,
			COALESCE(SUM(CASE WHEN state = 'idle' THEN 1 ELSE 0 END), 0) AS idle_connections,
			COUNT(*) AS total_connections
		FROM pg_stat_activity
		WHERE pid != pg_backend_pid()
	`

	var active, idle, total int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err := db.QueryRowContext(ctx, query).Scan(&active, &idle, &total)
	if err != nil {
		slog.Error("[PostgreSQL] Connection Stats Query Error", "err", err)
	}
	return active, idle, total, err
}

// CollectPgDatabases fetches list of databases
func (c *PgRepository) CollectPgDatabases(ctx context.Context, db *sql.DB) ([]string, error) {
	query := `
		SELECT /* SQL_OPTIMA */   datname 
		FROM pg_database 
		WHERE datistemplate = false 
		ORDER BY datname
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		slog.Error("[PostgreSQL] Databases Query Error", "err", err)
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err == nil {
			results = append(results, dbName)
		}
	}
	return results, nil
}
