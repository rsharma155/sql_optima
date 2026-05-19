// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL CPU dashboard queries — execution time by database and top statements (pg_stat_statements).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PgCpuDbRow is one row of cumulative execution time per database (proxy for CPU time share).
type PgCpuDbRow struct {
	Datname         string  `json:"datname"`
	TotalExecTimeMs float64 `json:"total_exec_time_ms"`
}

// PgTopCpuQueryRow is a top statement by total_exec_time (CPU dashboard table).
type PgTopCpuQueryRow struct {
	CapturedAt      time.Time `json:"captured_at"`
	UserName        string    `json:"user_name"`
	QueryID         int64     `json:"queryid"`
	Query           string    `json:"query"`
	TotalExecTimeMs float64   `json:"total_exec_time"`
	Calls           int64     `json:"calls"`
	AvgMs           float64   `json:"avg_ms"`
}

// GetCpuTimeByDatabase returns total_exec_time summed per database (same shape as mv_pg_cpu_by_db).
func (c *PgRepository) GetCpuTimeByDatabase(ctx context.Context, instanceName string) ([]PgCpuDbRow, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	if !c.GetPgssSupported(instanceName) {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	version := c.GetPgVersion(ctx, instanceName)
	timeCol := "total_exec_time"
	if version < 130000 {
		timeCol = "total_time"
	}

	q := fmt.Sprintf(`SELECT /* SQL_OPTIMA */   d.datname::text AS datname,
			SUM(s.%s)::float8 AS total_exec_time_ms
		FROM pg_stat_statements s
		JOIN pg_database d ON d.oid = s.dbid
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE %s
		GROUP BY d.datname
		ORDER BY total_exec_time_ms DESC`, timeCol, buildPgStatStatementsFilters())

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgCpuDbRow
	for rows.Next() {
		var r PgCpuDbRow
		if err := rows.Scan(&r.Datname, &r.TotalExecTimeMs); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetTopCpuQueries returns top statements by total_exec_time (same shape as mv_pg_top_cpu_queries).
func (c *PgRepository) GetTopCpuQueries(ctx context.Context, instanceName string, limit int) ([]PgTopCpuQueryRow, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	if !c.GetPgssSupported(instanceName) {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	version := c.GetPgVersion(ctx, instanceName)
	timeCol := "total_exec_time"
	if version < 130000 {
		timeCol = "total_time"
	}

	q := fmt.Sprintf(`/* SQL_OPTIMA */ SELECT   s.queryid,
			now()::timestamptz AS captured_at,
			COALESCE(r.rolname, '') AS user_name,
			LEFT(s.query, 400) AS query,
			s.%s::float8,
			s.calls::bigint,
			CASE WHEN s.calls > 0 THEN (s.%s / s.calls)::float8 ELSE 0 END AS avg_ms
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE %s
		ORDER BY s.%s DESC
		LIMIT $1`, timeCol, timeCol, buildPgStatStatementsFilters(), timeCol)

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PgTopCpuQueryRow
	for rows.Next() {
		var r PgTopCpuQueryRow
		if err := rows.Scan(&r.QueryID, &r.CapturedAt, &r.UserName, &r.Query, &r.TotalExecTimeMs, &r.Calls, &r.AvgMs); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
