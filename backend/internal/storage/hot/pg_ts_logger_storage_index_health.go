// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL-specific Timescale queries for the storage index health dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
)

func (tl *TimescaleLogger) sihPgHighScanTableCount(ctx context.Context, engine, serverID, from, to string, f SIHFilters) int {
	args := []interface{}{engine, serverID, from, to}
	where := `engine=$1 AND server_id=$2 AND time >= $3::timestamptz AND time <= $4::timestamptz`
	argN := 5
	if len(f.DBNames) > 0 {
		where += fmt.Sprintf(" AND db_name = ANY($%d)", argN)
		args = append(args, f.DBNames)
		argN++
	}
	if len(f.SchemaNames) > 0 {
		where += fmt.Sprintf(" AND schema_name = ANY($%d)", argN)
		args = append(args, f.SchemaNames)
		argN++
	}
	if f.TableLike != "" {
		where += fmt.Sprintf(" AND table_name ILIKE $%d", argN)
		args = append(args, "%"+f.TableLike+"%")
	}
	q := fmt.Sprintf(`
		SELECT COUNT(*)::int
		FROM (
			SELECT db_name, schema_name, table_name,
			       (SUM(COALESCE(seq_scans,0))::float8 / NULLIF(SUM(COALESCE(idx_scans,0))::float8, 0)) AS ratio
			FROM monitor.table_usage_stats
			WHERE %s
			GROUP BY db_name, schema_name, table_name
		) t
		WHERE COALESCE(t.ratio, 0) > 5
	`, where)
	var n int
	_ = tl.pool.QueryRow(ctx, q, args...).Scan(&n)
	return n
}

func (tl *TimescaleLogger) sihPgTopScans(ctx context.Context, engine, serverID, from, to string, f SIHFilters) ([]StorageIndexHealthTopRow, error) {
	args := []interface{}{engine, serverID, from, to}
	where := `engine = $1 AND server_id = $2 AND time >= $3::timestamptz AND time <= $4::timestamptz`
	where, args, _ = sihAppendFilters(where, args, f, 5)
	q := fmt.Sprintf(`
		SELECT db_name, schema_name, table_name,
		       SUM(COALESCE(seq_scans,0))::float8 AS v,
		       SUM(COALESCE(idx_scans,0))::float8 AS v2
		FROM monitor.table_usage_stats
		WHERE %s
		GROUP BY db_name, schema_name, table_name
		ORDER BY v DESC
		LIMIT 10
	`, where)
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.table_usage_stats")
		}
		return nil, err
	}
	defer rows.Close()
	var topScans []StorageIndexHealthTopRow
	for rows.Next() {
		var r StorageIndexHealthTopRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.Value, &r.Value2); err == nil {
			topScans = append(topScans, r)
		}
	}
	return topScans, nil
}

func (tl *TimescaleLogger) sihPgSeekScanLookup(ctx context.Context, engine, serverID, from, to string, f SIHFilters) ([]StorageIndexHealthSeekScanLookupRow, error) {
	args := []interface{}{engine, serverID, from, to}
	where := `engine=$1 AND server_id=$2 AND time >= $3::timestamptz AND time <= $4::timestamptz`
	argN := 5
	if len(f.DBNames) > 0 {
		where += fmt.Sprintf(" AND db_name = ANY($%d)", argN)
		args = append(args, f.DBNames)
		argN++
	}
	if len(f.SchemaNames) > 0 {
		where += fmt.Sprintf(" AND schema_name = ANY($%d)", argN)
		args = append(args, f.SchemaNames)
		argN++
	}
	if f.TableLike != "" {
		where += fmt.Sprintf(" AND table_name ILIKE $%d", argN)
		args = append(args, "%"+f.TableLike+"%")
	}
	q := fmt.Sprintf(`
		SELECT db_name, schema_name, table_name,
		       SUM(COALESCE(idx_scans,0))::float8 AS seeks,
		       SUM(COALESCE(seq_scans,0))::float8 AS scans,
		       0::float8 AS lookups
		FROM monitor.table_usage_stats
		WHERE %s
		GROUP BY db_name, schema_name, table_name
		ORDER BY (SUM(COALESCE(idx_scans,0)) + SUM(COALESCE(seq_scans,0))) DESC
		LIMIT 10
	`, where)
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.table_usage_stats")
		}
		return nil, err
	}
	defer rows.Close()
	var seekScan []StorageIndexHealthSeekScanLookupRow
	for rows.Next() {
		var r StorageIndexHealthSeekScanLookupRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.Seeks, &r.Scans, &r.Lookups); err == nil {
			seekScan = append(seekScan, r)
		}
	}
	return seekScan, nil
}

func (tl *TimescaleLogger) sihPgHighScanTables(ctx context.Context, engine, serverID, from, to string, f SIHFilters) ([]StorageIndexHealthTopRow, error) {
	args := []interface{}{engine, serverID, from, to}
	where := `engine=$1 AND server_id=$2 AND time >= $3::timestamptz AND time <= $4::timestamptz`
	n := 5
	if len(f.DBNames) > 0 {
		where += fmt.Sprintf(" AND db_name = ANY($%d)", n)
		args = append(args, f.DBNames)
		n++
	}
	if len(f.SchemaNames) > 0 {
		where += fmt.Sprintf(" AND schema_name = ANY($%d)", n)
		args = append(args, f.SchemaNames)
		n++
	}
	if f.TableLike != "" {
		where += fmt.Sprintf(" AND table_name ILIKE $%d", n)
		args = append(args, "%"+f.TableLike+"%")
	}
	q := fmt.Sprintf(`
		SELECT db_name, schema_name, table_name,
		       SUM(COALESCE(seq_scans,0))::float8 AS seq_scans,
		       SUM(COALESCE(idx_scans,0))::float8 AS idx_scans
		FROM monitor.table_usage_stats
		WHERE %s
		GROUP BY db_name, schema_name, table_name
		HAVING SUM(COALESCE(seq_scans,0)) > 0
		ORDER BY (SUM(COALESCE(seq_scans,0))::float8 / NULLIF(SUM(COALESCE(idx_scans,0))::float8,0)) DESC,
		         seq_scans DESC
		LIMIT 50
	`, where)
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.table_usage_stats")
		}
		return nil, err
	}
	defer rows.Close()
	var highScan []StorageIndexHealthTopRow
	for rows.Next() {
		var r StorageIndexHealthTopRow
		var scans, seeks float64
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &scans, &seeks); err == nil {
			r.Value = scans
			r.Value2 = seeks
			highScan = append(highScan, r)
		}
	}
	return highScan, nil
}
