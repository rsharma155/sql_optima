// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server–specific Timescale queries for the storage index health dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
)

func (tl *TimescaleLogger) sihMssqlHighScanTableCount(ctx context.Context, engine, serverID, from, to string, f SIHFilters) int {
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
		argN++
	}
	q := fmt.Sprintf(`
		SELECT COUNT(*)::int
		FROM (
			SELECT db_name, schema_name, table_name,
			       (SUM(COALESCE(scans,0))::float8 / NULLIF(SUM(COALESCE(seeks,0))::float8, 0)) AS ratio
			FROM monitor.index_usage_stats
			WHERE %s
			GROUP BY db_name, schema_name, table_name
		) t
		WHERE COALESCE(t.ratio, 0) > 10
	`, where)
	var n int
	_ = tl.pool.QueryRow(ctx, q, args...).Scan(&n)
	return n
}

func (tl *TimescaleLogger) sihMssqlTopScans(ctx context.Context, engine, serverID, from, to string, f SIHFilters) ([]StorageIndexHealthTopRow, error) {
	args := []interface{}{engine, serverID, from, to}
	where := `engine = $1 AND server_id = $2 AND time >= $3::timestamptz AND time <= $4::timestamptz`
	where, args, _ = sihAppendFilters(where, args, f, 5)
	q := fmt.Sprintf(`
		SELECT db_name, schema_name, table_name,
		       SUM(COALESCE(scans,0))::float8 AS v,
		       SUM(COALESCE(seeks,0))::float8 AS v2
		FROM monitor.index_usage_stats
		WHERE %s
		GROUP BY db_name, schema_name, table_name
		ORDER BY v DESC
		LIMIT 10
	`, where)
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.index_usage_stats")
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

func (tl *TimescaleLogger) sihMssqlSeekScanLookup(ctx context.Context, engine, serverID, from, to string, f SIHFilters) ([]StorageIndexHealthSeekScanLookupRow, error) {
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
		argN++
	}

	q := fmt.Sprintf(`
		SELECT db_name, schema_name, table_name,
		       SUM(COALESCE(seeks,0))::float8 AS seeks,
		       SUM(COALESCE(scans,0))::float8 AS scans,
		       SUM(COALESCE(lookups,0))::float8 AS lookups
		FROM monitor.index_usage_stats
		WHERE %s
		GROUP BY db_name, schema_name, table_name
		ORDER BY (SUM(COALESCE(seeks,0)) + SUM(COALESCE(scans,0)) + SUM(COALESCE(lookups,0))) DESC
		LIMIT 10
	`, where)
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.index_usage_stats")
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

func (tl *TimescaleLogger) sihMssqlHighScanTables(ctx context.Context, engine, serverID, from, to string, f SIHFilters) ([]StorageIndexHealthTopRow, error) {
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
		n++
	}
	q := fmt.Sprintf(`
		SELECT db_name, schema_name, table_name,
		       SUM(COALESCE(scans,0))::float8 AS scans,
		       SUM(COALESCE(seeks,0))::float8 AS seeks
		FROM monitor.index_usage_stats
		WHERE %s
		GROUP BY db_name, schema_name, table_name
		HAVING SUM(COALESCE(scans,0)) > 0
		ORDER BY (SUM(COALESCE(scans,0))::float8 / NULLIF(SUM(COALESCE(seeks,0))::float8,0)) DESC,
		         scans DESC
		LIMIT 50
	`, where)
	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		if isMissingRelation(err) {
			return nil, schemaMissingErr("monitor.index_usage_stats")
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
