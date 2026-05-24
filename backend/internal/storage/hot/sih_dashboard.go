// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Timescale-backed Storage/Index Health dashboard query/aggregation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (tl *TimescaleLogger) QueryStorageIndexHealthDashboard(ctx context.Context, engine, serverID, from, to string, f SIHFilters) (*StorageIndexHealthDashboard, error) {
	if f.DBNames != nil {
		f.DBNames = normalizeCSV(f.DBNames)
	}
	if f.SchemaNames != nil {
		f.SchemaNames = normalizeCSV(f.SchemaNames)
	}
	f.TableLike = strings.TrimSpace(f.TableLike)

	// Growth chart: table_size_history is sampled every 6h; for short UI windows (1h/24h) daily buckets
	// often look empty. Widen the growth query to at least 7d ending at `to` when the range is < 48h.
	growthFrom := from
	if fromTs, err1 := time.Parse(time.RFC3339, from); err1 == nil {
		if toTs, err2 := time.Parse(time.RFC3339, to); err2 == nil {
			if toTs.Sub(fromTs) < 48*time.Hour {
				growthFrom = toTs.Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)
			}
		}
	}

	// ---------------- KPIs ----------------
	kpis := StorageIndexHealthKPI{}
	{
		// Unused index count in window: 0 reads, >0 updates (and not PK)
		args := []interface{}{engine, serverID, from, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
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
			where += fmt.Sprintf(" AND table_name = $%d", argN)
			args = append(args, f.TableLike)
		}

		q := fmt.Sprintf(`
			SELECT COUNT(*)::int
			FROM (
				SELECT db_name, schema_name, table_name, index_name
				FROM monitor.index_usage_stats
				WHERE %s
				GROUP BY db_name, schema_name, table_name, index_name
				HAVING SUM(COALESCE(seeks,0) + COALESCE(scans,0) + COALESCE(lookups,0)) = 0
				   AND BOOL_OR(COALESCE(is_pk,false)) = false
				   AND (
				     ($1::text = 'postgres' AND MAX(COALESCE(index_size_mb,0)) >= 0.01)
				     OR ($1::text <> 'postgres' AND SUM(COALESCE(updates,0)) > 0)
				   )
			) t
		`, where)
		if err := tl.pool.QueryRow(ctx, q, args...).Scan(&kpis.UnusedIndexCount); err != nil {
			if isMissingRelation(err) {
				return nil, schemaMissingErr("monitor.index_usage_stats")
			}
			return nil, err
		}
	}
	{
		// Index write overhead %
		args := []interface{}{engine, serverID, from, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
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
			where += fmt.Sprintf(" AND table_name = $%d", argN)
			args = append(args, f.TableLike)
		}
		q := fmt.Sprintf(`
			SELECT COALESCE(
				(SUM(COALESCE(updates,0))::float8 / NULLIF(SUM(COALESCE(updates,0) + COALESCE(seeks,0) + COALESCE(scans,0) + COALESCE(lookups,0))::float8, 0)) * 100.0,
				0
			) AS pct
			FROM monitor.index_usage_stats
			WHERE %s
		`, where)
		if err := tl.pool.QueryRow(ctx, q, args...).Scan(&kpis.IndexWriteOverheadPct); err != nil {
			if isMissingRelation(err) {
				return nil, schemaMissingErr("monitor.index_usage_stats")
			}
			return nil, err
		}
	}

	// High scan tables count (engine-specific meaning)
	if engine == "postgres" {
		kpis.HighScanTableCount = tl.sihPgHighScanTableCount(ctx, engine, serverID, from, to, f)
	} else {
		kpis.HighScanTableCount = tl.sihSqlServerHighScanTableCount(ctx, engine, serverID, from, to, f)
	}

	// Fastest growing table (7d) - best effort, within provided window.
	{
		args := []interface{}{engine, serverID, to}
		where := `engine = $1 AND server_id = $2::uuid AND capture_timestamp <= $3::timestamptz`
		where, args, _ = sihAppendFilters(where, args, f, 4)
		q := fmt.Sprintf(`
			WITH latest AS (
			  SELECT DISTINCT ON (db_name, schema_name, table_name)
			    db_name, schema_name, table_name, table_size_mb, capture_timestamp
			  FROM monitor.table_size_history
			  WHERE %s
			  ORDER BY db_name, schema_name, table_name, capture_timestamp DESC
			),
			earliest AS (
			  SELECT DISTINCT ON (db_name, schema_name, table_name)
			    db_name, schema_name, table_name, table_size_mb, capture_timestamp
			  FROM monitor.table_size_history
			  WHERE %s AND capture_timestamp >= ($3::timestamptz - INTERVAL '7 days')
			  ORDER BY db_name, schema_name, table_name, capture_timestamp ASC
			)
			SELECT COALESCE(l.db_name || '.' || l.schema_name || '.' || l.table_name, ''),
			       COALESCE(
			         CASE WHEN e.table_size_mb > 0
			           THEN ((l.table_size_mb - e.table_size_mb) / e.table_size_mb) * 100.0
			           ELSE 0 END,
			         0
			       )::float8
			FROM latest l
			JOIN earliest e USING (db_name, schema_name, table_name)
			ORDER BY (l.table_size_mb - e.table_size_mb) DESC NULLS LAST
			LIMIT 1
		`, where, where)
		_ = tl.pool.QueryRow(ctx, q, args...).Scan(&kpis.FastestGrowingTable, &kpis.FastestGrowingTableGrowthPct7d)
	}

	// ---------------- Top Scans ----------------
	var topScans []StorageIndexHealthTopRow
	var err error
	if engine == "postgres" {
		topScans, err = tl.sihPgTopScans(ctx, engine, serverID, from, to, f)
	} else {
		topScans, err = tl.sihSqlServerTopScans(ctx, engine, serverID, from, to, f)
	}
	if err != nil {
		return nil, err
	}

	// ---------------- Row 2 Right: Seek vs Scan vs Lookup (SQL Server) or Idx vs Seq scans (Postgres tables) ----------------
	var seekScan []StorageIndexHealthSeekScanLookupRow
	if engine == "sqlserver" {
		seekScan, err = tl.sihSqlServerSeekScanLookup(ctx, engine, serverID, from, to, f)
		if err != nil {
			return nil, err
		}
	} else if engine == "postgres" {
		seekScan, err = tl.sihPgSeekScanLookup(ctx, engine, serverID, from, to, f)
		if err != nil {
			return nil, err
		}
	}

	// ---------------- Largest tables/indexes (latest snapshot) ----------------
	var largestTables []StorageIndexHealthTopRow
	{
		args := []interface{}{engine, serverID, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp <= $3::timestamptz`
		where, args, _ = sihAppendFilters(where, args, f, 4)
		// Prefer monitor.table_usage_stats as it is 15m cadence vs 6h for table_size_history
		q := fmt.Sprintf(`
			SELECT db_name, schema_name, table_name, 
			       COALESCE(table_size_mb,0)::float8 AS v, 
			       COALESCE(index_size_mb,0)::float8 AS v2,
			       COALESCE(row_count,0)::float8 AS rows
			FROM (
				SELECT DISTINCT ON (db_name, schema_name, table_name)
					db_name, schema_name, table_name, table_size_mb, index_size_mb, row_count, capture_timestamp
				FROM monitor.table_usage_stats
				WHERE %s
				ORDER BY db_name, schema_name, table_name, capture_timestamp DESC
			) t
			ORDER BY v DESC
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
		for rows.Next() {
			var r StorageIndexHealthTopRow
			var rc float64
			if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.Value, &r.Value2, &rc); err == nil {
				// We can store row count in Value field if we adjust others, or just use Value as Total Size.
				// Total Size = Table Size + Index Size
				r.Value = r.Value + r.Value2
				largestTables = append(largestTables, r)
			}
		}
		// Fallback to table_size_history if usage stats empty (e.g. just started)
		if len(largestTables) == 0 {
			q2 := fmt.Sprintf(`
				SELECT db_name, schema_name, table_name, 
				       COALESCE(table_size_mb,0)::float8 AS v, 
				       COALESCE(index_size_mb,0)::float8 AS v2,
				       COALESCE(row_count,0)::float8 AS rows
				FROM (
					SELECT DISTINCT ON (db_name, schema_name, table_name)
						db_name, schema_name, table_name, table_size_mb, index_size_mb, row_count, capture_timestamp
					FROM monitor.table_size_history
					WHERE %s
					ORDER BY db_name, schema_name, table_name, capture_timestamp DESC
				) t
				ORDER BY v DESC
				LIMIT 50
				`, where)
			rows2, _ := tl.pool.Query(ctx, q2, args...)
			if rows2 != nil {
				defer rows2.Close()
				for rows2.Next() {
					var r StorageIndexHealthTopRow
					var rc float64
					if err := rows2.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.Value, &r.Value2, &rc); err == nil {
						r.Value = r.Value + r.Value2
						largestTables = append(largestTables, r)
					}
				}
			}
		}
	}

	var largestIndexes []StorageIndexHealthTopRow
	{
		args := []interface{}{engine, serverID, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp <= $3::timestamptz AND COALESCE(is_pk,false) = false`
		where, args, _ = sihAppendFilters(where, args, f, 4)
		q := fmt.Sprintf(`
			SELECT db_name, schema_name, table_name, index_name, COALESCE(index_size_mb,0)::float8 AS v
			FROM (
				SELECT DISTINCT ON (db_name, schema_name, table_name, index_name)
					db_name, schema_name, table_name, index_name, index_size_mb, capture_timestamp
				FROM monitor.index_usage_stats
				WHERE %s
				ORDER BY db_name, schema_name, table_name, index_name, capture_timestamp DESC
			) t
			ORDER BY v DESC
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
		for rows.Next() {
			var r StorageIndexHealthTopRow
			if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.IndexName, &r.Value); err == nil {
				largestIndexes = append(largestIndexes, r)
			}
		}
	}

	// ---------------- Growth series (daily bucket, aggregated) ----------------
	var growth []StorageIndexHealthGrowthPoint
	{
		args := []interface{}{engine, serverID, growthFrom, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
		where, args, _ = sihAppendFilters(where, args, f, 5)
		// Use DISTINCT ON per (day-bucket, table) to get the latest snapshot per table per day
		// before summing; otherwise multiple intra-day samples inflate the total.
		q := fmt.Sprintf(`
			SELECT bucket,
			       SUM(table_size_mb)::float8,
			       SUM(index_size_mb)::float8
			FROM (
				SELECT DISTINCT ON (time_bucket('1 day', capture_timestamp), db_name, schema_name, table_name)
					time_bucket('1 day', capture_timestamp) AS bucket,
					COALESCE(table_size_mb,0) AS table_size_mb,
					COALESCE(index_size_mb,0) AS index_size_mb
				FROM monitor.table_size_history
				WHERE %s
				ORDER BY time_bucket('1 day', capture_timestamp), db_name, schema_name, table_name, capture_timestamp DESC
			) t
			GROUP BY bucket
			ORDER BY bucket ASC
			LIMIT 90
		`, where)
		rows, err := tl.pool.Query(ctx, q, args...)
		if err != nil {
			if isMissingRelation(err) {
				return nil, schemaMissingErr("monitor.table_size_history")
			}
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var p StorageIndexHealthGrowthPoint
			if err := rows.Scan(&p.Bucket, &p.TableSizeMB, &p.IndexSizeMB); err == nil {
				growth = append(growth, p)
			}
		}
		// If growth empty, try bucket from table_usage_stats (short window support)
		if len(growth) == 0 {
			q2 := fmt.Sprintf(`
				SELECT bucket,
					SUM(table_size_mb)::float8,
					SUM(index_size_mb)::float8
				FROM (
					SELECT DISTINCT ON (time_bucket('1 day', capture_timestamp), db_name, schema_name, table_name)
						time_bucket('1 day', capture_timestamp) AS bucket,
						COALESCE(table_size_mb,0) AS table_size_mb,
						COALESCE(index_size_mb,0) AS index_size_mb
					FROM monitor.table_usage_stats
					WHERE %s
					ORDER BY time_bucket('1 day', capture_timestamp), db_name, schema_name, table_name, capture_timestamp DESC
				) t
				GROUP BY bucket
				ORDER BY bucket ASC
			`, where)
			rows2, _ := tl.pool.Query(ctx, q2, args...)
			if rows2 != nil {
				defer rows2.Close()
				for rows2.Next() {
					var p StorageIndexHealthGrowthPoint
					if err := rows2.Scan(&p.Bucket, &p.TableSizeMB, &p.IndexSizeMB); err == nil {
						growth = append(growth, p)
					}
				}
			}
		}
	}

	// ---------------- Derived growth summary (daily deltas, % trends, projection) ----------------
	gSummary := StorageIndexHealthGrowthSummary{}
	{
		args := []interface{}{engine, serverID, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp <= $3::timestamptz AND capture_timestamp >= ($3::timestamptz - INTERVAL '31 days')`
		n := 4
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
			where += fmt.Sprintf(" AND table_name = $%d", n)
			args = append(args, f.TableLike)
		}

		// Prefer monitor.table_usage_stats for summary as it is more frequent
		q := fmt.Sprintf(`
			SELECT time_bucket('1 day', capture_timestamp) AS bucket,
			       SUM(COALESCE(table_size_mb,0))::float8 AS table_mb,
			       SUM(COALESCE(index_size_mb,0))::float8 AS index_mb,
			       SUM(COALESCE(row_count,0))::bigint AS rows
			FROM monitor.table_usage_stats
			WHERE %s
			GROUP BY bucket
			ORDER BY bucket ASC
		`, where)
		rows, err := tl.pool.Query(ctx, q, args...)
		if err != nil {
			if isMissingRelation(err) {
				return nil, schemaMissingErr("monitor.table_usage_stats")
			}
			return nil, err
		}
		defer rows.Close()

		type pt struct {
			bucket   time.Time
			tableMB  float64
			indexMB  float64
			rowCount int64
		}
		var pts []pt
		for rows.Next() {
			var p pt
			if err := rows.Scan(&p.bucket, &p.tableMB, &p.indexMB, &p.rowCount); err == nil {
				pts = append(pts, p)
			}
		}
		// Fallback to table_size_history if no usage stats
		if len(pts) == 0 {
			q2 := fmt.Sprintf(`
				SELECT time_bucket('1 day', capture_timestamp) AS bucket,
					SUM(COALESCE(table_size_mb,0))::float8 AS table_mb,
					SUM(COALESCE(index_size_mb,0))::float8 AS index_mb,
					SUM(COALESCE(row_count,0))::bigint AS rows
				FROM monitor.table_size_history
				WHERE %s
				GROUP BY bucket
				ORDER BY bucket ASC
			`, where)
			rows2, _ := tl.pool.Query(ctx, q2, args...)
			if rows2 != nil {
				defer rows2.Close()
				for rows2.Next() {
					var p pt
					if err := rows2.Scan(&p.bucket, &p.tableMB, &p.indexMB, &p.rowCount); err == nil {
						pts = append(pts, p)
					}
				}
			}
		}
		if len(pts) > 0 {
			last := pts[len(pts)-1]
			gSummary.CurrentTableMB = last.tableMB
			gSummary.CurrentIndexMB = last.indexMB
			gSummary.CurrentRowCount = last.rowCount

			if len(pts) >= 2 {
				prev := pts[len(pts)-2]
				gSummary.DailyGrowthMB = last.tableMB - prev.tableMB
				idxDaily := last.indexMB - prev.indexMB
				if idxDaily > 0 {
					gSummary.ProjectedIndexMB90d = last.indexMB + idxDaily*90.0
				} else {
					gSummary.ProjectedIndexMB90d = last.indexMB
				}
			} else {
				gSummary.DailyGrowthMB = 0
				gSummary.ProjectedIndexMB90d = last.indexMB
			}

			findBaseline := func(days int) (float64, bool) {
				target := last.bucket.Add(-time.Duration(days) * 24 * time.Hour)
				// pick the point with bucket <= target that's closest to target (scan from end).
				for i := len(pts) - 1; i >= 0; i-- {
					if !pts[i].bucket.After(target) {
						return pts[i].tableMB, true
					}
				}
				// fallback to earliest
				if len(pts) > 0 {
					return pts[0].tableMB, true
				}
				return 0, false
			}
			if base7, ok := findBaseline(7); ok && base7 > 0 {
				gSummary.Growth7dPct = ((last.tableMB - base7) / base7) * 100.0
			}
			if base30, ok := findBaseline(30); ok && base30 > 0 {
				gSummary.Growth30dPct = ((last.tableMB - base30) / base30) * 100.0
			}

			// Project 90 days from daily growth (table).
			if gSummary.DailyGrowthMB > 0 {
				gSummary.ProjectedTableMB90d = last.tableMB + gSummary.DailyGrowthMB*90.0
			} else {
				gSummary.ProjectedTableMB90d = last.tableMB
			}
		}
	}

	// ---------------- GRID 1: Unused indexes ----------------
	var unused []StorageIndexHealthTopRow
	{
		args := []interface{}{engine, serverID, from, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp >= $3::timestamptz AND capture_timestamp <= $4::timestamptz`
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
			where += fmt.Sprintf(" AND table_name = $%d", n)
			args = append(args, f.TableLike)
		}
		q := fmt.Sprintf(`
			WITH win AS (
			  SELECT db_name, schema_name, table_name, index_name,
			         SUM(COALESCE(seeks,0)+COALESCE(scans,0)+COALESCE(lookups,0)) AS reads,
			         SUM(COALESCE(updates,0)) AS updates
			  FROM monitor.index_usage_stats
			  WHERE %s
			  GROUP BY db_name, schema_name, table_name, index_name
			),
			latest AS (
			  SELECT DISTINCT ON (db_name, schema_name, table_name, index_name)
			        db_name, schema_name, table_name, index_name,
			        COALESCE(index_size_mb,0)::float8 AS size_mb,
			        last_user_seek
			  FROM monitor.index_usage_stats
			  WHERE engine=$1 AND server_id=$2::uuid AND capture_timestamp <= $4::timestamptz
			  ORDER BY db_name, schema_name, table_name, index_name, capture_timestamp DESC
			)
			SELECT w.db_name, w.schema_name, w.table_name, w.index_name,
			       COALESCE(l.size_mb,0)::float8 AS size_mb,
			       w.updates::float8 AS updates,
			       l.last_user_seek
			FROM win w
			LEFT JOIN latest l USING (db_name, schema_name, table_name, index_name)
			WHERE w.reads = 0 AND (
				($1::text = 'postgres' AND COALESCE(l.size_mb,0) >= 0.01)
				OR ($1::text <> 'postgres' AND w.updates > 0)
			)
			ORDER BY size_mb DESC, updates DESC
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
		for rows.Next() {
			var r StorageIndexHealthTopRow
			var size, updates float64
			var lastSeek sql.NullTime
			if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.IndexName, &size, &updates, &lastSeek); err == nil {
				r.Value = updates
				r.Value2 = size
				if lastSeek.Valid {
					t := lastSeek.Time
					r.LastUserSeek = &t
				}
				unused = append(unused, r)
			}
		}
	}

	// ---------------- GRID 2: High scan tables ----------------
	var highScan []StorageIndexHealthTopRow
	if engine == "postgres" {
		highScan, err = tl.sihPgHighScanTables(ctx, engine, serverID, from, to, f)
	} else {
		highScan, err = tl.sihSqlServerHighScanTables(ctx, engine, serverID, from, to, f)
	}
	if err != nil {
		return nil, err
	}

	// ---------------- GRID 3: Duplicate / overlapping index candidates ----------------
	var dup []map[string]interface{}
	if engine == "sqlserver" || engine == "postgres" {
		dup, err = tl.sihDuplicateIndexCandidates(ctx, engine, serverID, to, f)
		if err != nil {
			return nil, err
		}
	}

	// ---------------- Top Growth Tables (7d) ----------------
	var topGrowth []StorageIndexHealthTopRow
	{
		// Calculate growth per table over the last 7 days
		args := []interface{}{engine, serverID, to}
		where := `engine=$1 AND server_id=$2::uuid AND capture_timestamp <= $3::timestamptz AND capture_timestamp >= ($3::timestamptz - INTERVAL '7 days')`
		n := 4
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
			where += fmt.Sprintf(" AND table_name = $%d", n)
			args = append(args, f.TableLike)
		}

		q := fmt.Sprintf(`
			SELECT db_name, schema_name, table_name, 
			       (MAX(table_size_mb) - MIN(table_size_mb)) as growth_mb,
			       MIN(table_size_mb) as base_mb
			FROM monitor.table_usage_stats
			WHERE %s
			GROUP BY db_name, schema_name, table_name
			HAVING (MAX(table_size_mb) - MIN(table_size_mb)) > 0
			ORDER BY growth_mb DESC
			LIMIT 10
		`, where)
		rows, err := tl.pool.Query(ctx, q, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var r StorageIndexHealthTopRow
				if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.Value, &r.Value2); err == nil {
					topGrowth = append(topGrowth, r)
				}
			}
		}
	}

	// Final KPI adjustments from summaries
	kpis.TotalDBSizeMB = gSummary.CurrentTableMB + gSummary.CurrentIndexMB
	kpis.Growth7dPct = gSummary.Growth7dPct
	kpis.ForecastTableMB90d = gSummary.ProjectedTableMB90d + gSummary.ProjectedIndexMB90d
	for _, u := range unused {
		kpis.UnusedIndexMB += u.Value2
	}

		// ---------------- Collector Error Check ----------------
	var collectorErr string
	if engine == "sqlserver" {
		_ = tl.pool.QueryRow(ctx, "SELECT last_error FROM sqlserver_collector_instance_state WHERE server_id = $1::uuid", serverID).Scan(&collectorErr)
	}

	return &StorageIndexHealthDashboard{
		Engine: engine, Instance: serverID, From: from, To: to,
		CollectorError:      collectorErr,
		KPIs:                kpis,
		TopScans:            topScans,
		SeekScanLookup:      seekScan,
		LargestTables:       largestTables,
		LargestIndexes:      largestIndexes,
		TopGrowthTables:     topGrowth,
		Growth:              growth,
		GrowthSummary:       gSummary,
		UnusedIndexes:       unused,
		HighScanTables:      highScan,
		RawDuplicateIndexes: dup,
	}, nil
}
