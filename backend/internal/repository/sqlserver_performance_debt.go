// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Performance debt calculation and tracking over time.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type PerformanceDebtUnusedIndex struct {
	DatabaseName    string
	TableName       string
	IndexName       string
	UserSeeks       int64
	UserScans       int64
	UserLookups     int64
	UserUpdates     int64
	TotalReads      int64
	ServerStartTime time.Time // resets to zero on every SQL Server restart
	IndexCreateDate time.Time // used to suppress newly created indexes
}

func (c *SqlServerRepository) FetchUnusedIndexes(ctx context.Context, instanceName, databaseName string, minUpdates int64, limit int) ([]PerformanceDebtUnusedIndex, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}
	if minUpdates <= 0 {
		minUpdates = 1000
	}

	// NOTE: Must be executed in the target database context because dm_db_index_usage_stats is per-db.
	// COALESCE is required so indexes with no row in dm_db_index_usage_stats (never accessed since
	// last restart) are still returned. The old s.user_seeks = 0 pattern silently drops those rows.
	q := fmt.Sprintf(`
		/* SQL_OPTIMA */
		USE %s;
		SELECT /* SQL_OPTIMA */ TOP (%d)
			DB_NAME() AS database_name,
			OBJECT_NAME(i.object_id) AS table_name,
			i.name AS index_name,
			COALESCE(s.user_seeks,0) AS user_seeks,
			COALESCE(s.user_scans,0) AS user_scans,
			COALESCE(s.user_lookups,0) AS user_lookups,
			COALESCE(s.user_updates,0) AS user_updates,
			(COALESCE(s.user_seeks,0)+COALESCE(s.user_scans,0)+COALESCE(s.user_lookups,0)) AS total_reads,
			(SELECT TOP 1 sqlserver_start_time FROM sys.dm_os_sys_info) AS server_start_time,
			o.create_date AS index_create_date
		FROM sys.indexes i
		JOIN sys.objects o ON o.object_id = i.object_id
		LEFT JOIN sys.dm_db_index_usage_stats s
			ON s.object_id = i.object_id
		   AND s.index_id = i.index_id
		   AND s.database_id = DB_ID()
		WHERE i.index_id > 1
		  AND i.is_primary_key = 0
		  AND i.is_unique_constraint = 0
		  AND i.is_disabled = 0
		  AND i.is_hypothetical = 0
		  AND i.type NOT IN (5, 6)
		  AND OBJECTPROPERTY(i.object_id, 'IsUserTable') = 1
		  AND COALESCE(s.user_seeks, 0) = 0
		  AND COALESCE(s.user_scans, 0) = 0
		  AND COALESCE(s.user_lookups, 0) = 0
		  AND COALESCE(s.user_updates, 0) >= %d
		ORDER BY COALESCE(s.user_updates,0) DESC;
	`, quoteDb(databaseName), limit, minUpdates)

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		slog.Error(fmt.Sprintf("[SQLSERVER] FetchUnusedIndexes error for %s/%s: %v", instanceName, databaseName, err))
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtUnusedIndex, 0, limit)
	for rows.Next() {
		var r PerformanceDebtUnusedIndex
		if err := rows.Scan(&r.DatabaseName, &r.TableName, &r.IndexName, &r.UserSeeks, &r.UserScans, &r.UserLookups, &r.UserUpdates, &r.TotalReads, &r.ServerStartTime, &r.IndexCreateDate); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtMissingIndex struct {
	ImprovementScore  float64 // composite ranking score (avg_total_user_cost * avg_user_impact * seeks+scans)
	AvgUserImpact     float64 // actual estimated improvement % from sys.dm_db_missing_index_group_stats (0–100)
	TableName         string
	EqualityColumns   string
	InequalityColumns string
	IncludedColumns   string
	CreateStatement   string
}

func (c *SqlServerRepository) FetchMissingIndexRecommendations(ctx context.Context, instanceName, databaseName string, limit int) ([]PerformanceDebtMissingIndex, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 25
	}

	q := fmt.Sprintf(`
		/* SQL_OPTIMA */
		USE %s;
		SELECT /* SQL_OPTIMA */ TOP (%d)
			(COALESCE(migs.avg_total_user_cost,0) * COALESCE(migs.avg_user_impact,0) * (COALESCE(migs.user_seeks,0) + COALESCE(migs.user_scans,0))) AS improvement_score,
			COALESCE(migs.avg_user_impact, 0) AS avg_user_impact,
			mid.statement AS table_name,
			COALESCE(mid.equality_columns,'') AS equality_columns,
			COALESCE(mid.inequality_columns,'') AS inequality_columns,
			COALESCE(mid.included_columns,'') AS included_columns,
			'CREATE INDEX [IX_SQLOPTIMA_' + CONVERT(VARCHAR, mid.index_handle) + '] ON ' + mid.statement + ' (' +
				ISNULL(mid.equality_columns, '') +
				CASE WHEN mid.equality_columns IS NOT NULL AND mid.inequality_columns IS NOT NULL THEN ',' ELSE '' END +
				ISNULL(mid.inequality_columns, '') + ')' +
				ISNULL(' INCLUDE (' + mid.included_columns + ')', '') AS create_statement
		FROM sys.dm_db_missing_index_group_stats migs
		JOIN sys.dm_db_missing_index_groups mig
		  ON migs.group_handle = mig.index_group_handle
		JOIN sys.dm_db_missing_index_details mid
		  ON mig.index_handle = mid.index_handle
		WHERE mid.database_id = DB_ID()
		  AND migs.user_seeks >= 100
		  AND (migs.avg_total_user_cost * migs.avg_user_impact * (migs.user_seeks + migs.user_scans)) >= 500
		ORDER BY improvement_score DESC;
	`, quoteDb(databaseName), limit)

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		slog.Error(fmt.Sprintf("[SQLSERVER] FetchMissingIndexRecommendations error for %s/%s: %v", instanceName, databaseName, err))
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtMissingIndex, 0, limit)
	for rows.Next() {
		var r PerformanceDebtMissingIndex
		if err := rows.Scan(&r.ImprovementScore, &r.AvgUserImpact, &r.TableName, &r.EqualityColumns, &r.InequalityColumns, &r.IncludedColumns, &r.CreateStatement); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtIndexFrag struct {
	SchemaName string
	TableName  string
	IndexName  string
	IndexID    int
	FragPct    float64
	PageCount  int64
}

func (c *SqlServerRepository) FetchIndexFragmentation(ctx context.Context, instanceName, databaseName string, minFragPct float64, minPages int64, limit int) ([]PerformanceDebtIndexFrag, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}
	if minFragPct <= 0 {
		minFragPct = 10 // collect both WARNING (10-30%) and CRITICAL (>30%) ranges
	}
	if minPages <= 0 {
		minPages = 100
	}

	q := fmt.Sprintf(`
		/* SQL_OPTIMA */
		USE %s;
		SELECT /* SQL_OPTIMA */ TOP (%d)
			SCHEMA_NAME(o.schema_id) AS schema_name,
			OBJECT_NAME(ps.object_id) AS table_name,
			COALESCE(i.name,'') AS index_name,
			ps.index_id,
			ps.avg_fragmentation_in_percent,
			ps.page_count
		FROM sys.dm_db_index_physical_stats(DB_ID(), NULL, NULL, NULL, 'LIMITED') ps
		JOIN sys.indexes i
		  ON ps.object_id = i.object_id AND ps.index_id = i.index_id
		JOIN sys.objects o
		  ON o.object_id = ps.object_id
		WHERE ps.index_id >= 0
		  AND ps.avg_fragmentation_in_percent >= %f
		  AND ps.page_count >= %d
		  AND i.is_disabled = 0
		  AND i.is_hypothetical = 0
		  AND OBJECTPROPERTY(ps.object_id, 'IsUserTable') = 1
		ORDER BY ps.avg_fragmentation_in_percent DESC;
	`, quoteDb(databaseName), limit, minFragPct, minPages)

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		slog.Error(fmt.Sprintf("[SQLSERVER] FetchIndexFragmentation error for %s/%s: %v", instanceName, databaseName, err))
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtIndexFrag, 0, limit)
	for rows.Next() {
		var r PerformanceDebtIndexFrag
		if err := rows.Scan(&r.SchemaName, &r.TableName, &r.IndexName, &r.IndexID, &r.FragPct, &r.PageCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtStaleStats struct {
	TableName           string
	StatsName           string
	LastUpdated         sql.NullTime
	Rows                int64
	ModificationCounter int64
	NoRecompute         bool
}

func (c *SqlServerRepository) FetchStaleStatistics(ctx context.Context, instanceName, databaseName string, limit int) ([]PerformanceDebtStaleStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}

	// SQL Server 2016+ uses a dynamic auto-update threshold:
	//   rows <= 25,000: 20% of rows
	//   rows > 25,000:  20% + sqrt(1000 * rows)
	// Also flag NO_RECOMPUTE stats and stats untouched for 30+ days.
	q := fmt.Sprintf(`
		/* SQL_OPTIMA */
		USE %s;
		SELECT /* SQL_OPTIMA */ TOP (%d)
			OBJECT_NAME(s.object_id) AS table_name,
			s.name AS stats_name,
			STATS_DATE(s.object_id, s.stats_id) AS last_updated,
			COALESCE(sp.rows,0) AS rows,
			COALESCE(sp.modification_counter,0) AS modification_counter,
			s.no_recompute
		FROM sys.stats s
		CROSS APPLY sys.dm_db_stats_properties(s.object_id, s.stats_id) sp
		WHERE OBJECTPROPERTY(s.object_id, 'IsUserTable') = 1
		  AND (
		    s.no_recompute = 1
		    OR (COALESCE(sp.rows,0) > 0 AND COALESCE(sp.modification_counter,0) >
		        CASE
		            WHEN COALESCE(sp.rows,0) <= 25000 THEN COALESCE(sp.rows,0) * 0.20
		            ELSE (0.20 * COALESCE(sp.rows,0)) + SQRT(1000.0 * COALESCE(sp.rows,0))
		        END)
		    OR STATS_DATE(s.object_id, s.stats_id) < DATEADD(DAY, -30, GETDATE())
		  )
		ORDER BY COALESCE(sp.modification_counter,0) DESC;
	`, quoteDb(databaseName), limit)

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		slog.Error(fmt.Sprintf("[SQLSERVER] FetchStaleStatistics error for %s/%s: %v", instanceName, databaseName, err))
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtStaleStats, 0, limit)
	for rows.Next() {
		var r PerformanceDebtStaleStats
		if err := rows.Scan(&r.TableName, &r.StatsName, &r.LastUpdated, &r.Rows, &r.ModificationCounter, &r.NoRecompute); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtFileGrowth struct {
	FileName        string
	FileType        int     // 0=data, 1=log
	SizeMB          float64
	MaxSizeMB       float64 // -1 = unlimited
	Growth          int64   // pages when fixed, percentage value when percent-based
	IsPercentGrowth bool
}

func (c *SqlServerRepository) FetchAutogrowthRisks(ctx context.Context, instanceName, databaseName string, limit int) ([]PerformanceDebtFileGrowth, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}

	q := fmt.Sprintf(`
		/* SQL_OPTIMA */
		USE %s;
		SELECT /* SQL_OPTIMA */ TOP (%d)
			name,
			type,
			size*8/1024.0 AS size_mb,
			CASE WHEN max_size IN (-1, 268435456) THEN -1 ELSE max_size*8/1024.0 END AS max_size_mb,
			growth,
			is_percent_growth
		FROM sys.database_files
		ORDER BY size DESC;
	`, quoteDb(databaseName), limit)
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		slog.Error(fmt.Sprintf("[SQLSERVER] FetchAutogrowthRisks error for %s/%s: %v", instanceName, databaseName, err))
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtFileGrowth, 0, limit)
	for rows.Next() {
		var r PerformanceDebtFileGrowth
		if err := rows.Scan(&r.FileName, &r.FileType, &r.SizeMB, &r.MaxSizeMB, &r.Growth, &r.IsPercentGrowth); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (c *SqlServerRepository) FetchVLFCount(ctx context.Context, instanceName, databaseName string) (int64, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := fmt.Sprintf(`
		/* SQL_OPTIMA */ 
		USE %s;
		SELECT /* SQL_OPTIMA */   COUNT(*) AS vlf_count
		FROM sys.dm_db_log_info(DB_ID());
	`, quoteDb(databaseName))
	var n int64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		slog.Error(fmt.Sprintf("[SQLSERVER] FetchVLFCount error for %s/%s: %v", instanceName, databaseName, err))
		return 0, err
	}
	return n, nil
}

func (c *SqlServerRepository) FetchLastFullBackupAgeHours(ctx context.Context, instanceName, databaseName string) (float64, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := `
		/* SQL_OPTIMA */ SELECT   TOP 1 DATEDIFF(MINUTE, backup_finish_date, GETUTCDATE()) / 60.0 AS age_hours
		FROM msdb.dbo.backupset
		WHERE database_name = @p1
		  AND type = 'D'
		ORDER BY backup_finish_date DESC;
	`
	var age sql.NullFloat64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, q, databaseName).Scan(&age); err != nil {
		if err == sql.ErrNoRows {
			return 999999, nil
		}
		return 0, err
	}
	if !age.Valid {
		return 999999, nil
	}
	return age.Float64, nil
}

func (c *SqlServerRepository) FetchFailedAgentJobs24h(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}
	q := fmt.Sprintf(`
		/* SQL_OPTIMA */ SELECT   TOP (%d)
			j.name AS job_name,
			msdb.dbo.agent_datetime(h.run_date, h.run_time) AS run_dt,
			h.run_status
		FROM msdb.dbo.sysjobs j
		JOIN msdb.dbo.sysjobhistory h
		  ON j.job_id = h.job_id
		WHERE h.step_id = 0
		  AND h.run_status = 0
		  AND msdb.dbo.agent_datetime(h.run_date, h.run_time) >= DATEADD(HOUR, -24, GETUTCDATE())
		ORDER BY run_dt DESC;
	`, limit)
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var name string
		var runDt time.Time
		var status int
		if err := rows.Scan(&name, &runDt, &status); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"job_name": name,
			"run_dt":   runDt,
			"status":   status,
		})
	}
	return out, rows.Err()
}

func (c *SqlServerRepository) FetchDisabledAgentJobs(ctx context.Context, instanceName string, limit int) ([]string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 200
	}
	q := fmt.Sprintf(`/* SQL_OPTIMA */ SELECT   TOP (%d) name FROM msdb.dbo.sysjobs WHERE enabled = 0 ORDER BY name`, limit)
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			continue
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (c *SqlServerRepository) FetchConfigValueInUse(ctx context.Context, instanceName string, name string) (int64, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := `SELECT /* SQL_OPTIMA */ value_in_use FROM sys.configurations WHERE name = @p1`
	var v int64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, q, name).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func (c *SqlServerRepository) FetchLastLogBackupAgeHours(ctx context.Context, instanceName, databaseName string) (float64, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := `
		/* SQL_OPTIMA */ SELECT TOP 1 DATEDIFF(MINUTE, backup_finish_date, GETUTCDATE()) / 60.0 AS age_hours
		FROM msdb.dbo.backupset
		WHERE database_name = @p1
		  AND type = 'L'
		ORDER BY backup_finish_date DESC;
	`
	var age sql.NullFloat64
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, q, databaseName).Scan(&age); err != nil {
		if err == sql.ErrNoRows {
			return 999999, nil
		}
		return 0, err
	}
	if !age.Valid {
		return 999999, nil
	}
	return age.Float64, nil
}

func (c *SqlServerRepository) FetchDatabaseRecoveryModel(ctx context.Context, instanceName, databaseName string) (string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return "", fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := `SELECT /* SQL_OPTIMA */ recovery_model_desc FROM sys.databases WHERE name = @p1`
	var model string
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, q, databaseName).Scan(&model); err != nil {
		return "", err
	}
	return model, nil
}

func quoteDb(dbName string) string {
	// Minimal quoting for USE <db>.
	// Disallow dangerous characters; caller supplies db from config anyway.
	clean := strings.ReplaceAll(dbName, "]", "")
	return "[" + clean + "]"
}
