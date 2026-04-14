// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Performance debt calculation and tracking over time.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

type PerformanceDebtUnusedIndex struct {
	DatabaseName string
	TableName    string
	IndexName    string
	UserSeeks    int64
	UserScans    int64
	UserLookups  int64
	UserUpdates  int64
	TotalReads   int64
}

func (c *MssqlRepository) FetchUnusedIndexes(instanceName, databaseName string, minUpdates int64, limit int) ([]PerformanceDebtUnusedIndex, error) {
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
	q := fmt.Sprintf(`
		USE %s;
		SELECT TOP (%d)
			DB_NAME() AS database_name,
			OBJECT_NAME(i.object_id) AS table_name,
			i.name AS index_name,
			COALESCE(s.user_seeks,0) AS user_seeks,
			COALESCE(s.user_scans,0) AS user_scans,
			COALESCE(s.user_lookups,0) AS user_lookups,
			COALESCE(s.user_updates,0) AS user_updates,
			(COALESCE(s.user_seeks,0)+COALESCE(s.user_scans,0)+COALESCE(s.user_lookups,0)) AS total_reads
		FROM sys.indexes i
		LEFT JOIN sys.dm_db_index_usage_stats s
			ON s.object_id = i.object_id
		   AND s.index_id = i.index_id
		   AND s.database_id = DB_ID()
		WHERE i.index_id > 1
		  AND i.is_primary_key = 0
		  AND i.is_unique_constraint = 0
		  AND i.is_disabled = 0
		  AND i.is_hypothetical = 0
		  AND (COALESCE(s.user_seeks,0)+COALESCE(s.user_scans,0)+COALESCE(s.user_lookups,0)) = 0
		  AND COALESCE(s.user_updates,0) >= %d
		ORDER BY COALESCE(s.user_updates,0) DESC;
	`, quoteDb(databaseName), limit, minUpdates)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[MSSQL] FetchUnusedIndexes error for %s/%s: %v", instanceName, databaseName, err)
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtUnusedIndex, 0, limit)
	for rows.Next() {
		var r PerformanceDebtUnusedIndex
		if err := rows.Scan(&r.DatabaseName, &r.TableName, &r.IndexName, &r.UserSeeks, &r.UserScans, &r.UserLookups, &r.UserUpdates, &r.TotalReads); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtMissingIndex struct {
	ImprovementScore float64
	TableName        string
	EqualityColumns  string
	InequalityCols   string
	IncludedColumns  string
}

func (c *MssqlRepository) FetchMissingIndexRecommendations(instanceName, databaseName string, limit int) ([]PerformanceDebtMissingIndex, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 25
	}

	q := fmt.Sprintf(`
		USE %s;
		SELECT TOP (%d)
			(COALESCE(migs.avg_total_user_cost,0) * COALESCE(migs.avg_user_impact,0) * (COALESCE(migs.user_seeks,0) + COALESCE(migs.user_scans,0))) AS improvement_score,
			mid.statement AS table_name,
			COALESCE(mid.equality_columns,'') AS equality_columns,
			COALESCE(mid.inequality_columns,'') AS inequality_columns,
			COALESCE(mid.included_columns,'') AS included_columns
		FROM sys.dm_db_missing_index_group_stats migs
		JOIN sys.dm_db_missing_index_groups mig
		  ON migs.group_handle = mig.index_group_handle
		JOIN sys.dm_db_missing_index_details mid
		  ON mig.index_handle = mid.index_handle
		WHERE mid.database_id = DB_ID()
		ORDER BY improvement_score DESC;
	`, quoteDb(databaseName), limit)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[MSSQL] FetchMissingIndexRecommendations error for %s/%s: %v", instanceName, databaseName, err)
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtMissingIndex, 0, limit)
	for rows.Next() {
		var r PerformanceDebtMissingIndex
		if err := rows.Scan(&r.ImprovementScore, &r.TableName, &r.EqualityColumns, &r.InequalityCols, &r.IncludedColumns); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtIndexFrag struct {
	TableName string
	IndexName string
	IndexID   int
	FragPct   float64
	PageCount int64
}

func (c *MssqlRepository) FetchIndexFragmentation(instanceName, databaseName string, minFragPct float64, minPages int64, limit int) ([]PerformanceDebtIndexFrag, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}
	if minFragPct <= 0 {
		minFragPct = 30
	}
	if minPages <= 0 {
		minPages = 1000
	}

	q := fmt.Sprintf(`
		USE %s;
		SELECT TOP (%d)
			OBJECT_NAME(ps.object_id) AS table_name,
			COALESCE(i.name,'') AS index_name,
			ps.index_id,
			ps.avg_fragmentation_in_percent,
			ps.page_count
		FROM sys.dm_db_index_physical_stats(DB_ID(), NULL, NULL, NULL, 'LIMITED') ps
		JOIN sys.indexes i
		  ON ps.object_id = i.object_id AND ps.index_id = i.index_id
		WHERE ps.index_id > 0
		  AND ps.avg_fragmentation_in_percent >= %f
		  AND ps.page_count >= %d
		  AND i.is_disabled = 0
		  AND i.is_hypothetical = 0
		ORDER BY ps.avg_fragmentation_in_percent DESC;
	`, quoteDb(databaseName), limit, minFragPct, minPages)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[MSSQL] FetchIndexFragmentation error for %s/%s: %v", instanceName, databaseName, err)
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtIndexFrag, 0, limit)
	for rows.Next() {
		var r PerformanceDebtIndexFrag
		if err := rows.Scan(&r.TableName, &r.IndexName, &r.IndexID, &r.FragPct, &r.PageCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtStaleStats struct {
	TableName            string
	StatsName            string
	LastUpdated          sql.NullTime
	Rows                 int64
	ModificationCounter  int64
}

func (c *MssqlRepository) FetchStaleStatistics(instanceName, databaseName string, modificationPct float64, limit int) ([]PerformanceDebtStaleStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}
	if modificationPct <= 0 {
		modificationPct = 0.20
	}

	q := fmt.Sprintf(`
		USE %s;
		SELECT TOP (%d)
			OBJECT_NAME(s.object_id) AS table_name,
			s.name AS stats_name,
			STATS_DATE(s.object_id, s.stats_id) AS last_updated,
			COALESCE(sp.rows,0) AS rows,
			COALESCE(sp.modification_counter,0) AS modification_counter
		FROM sys.stats s
		CROSS APPLY sys.dm_db_stats_properties(s.object_id, s.stats_id) sp
		WHERE COALESCE(sp.rows,0) > 0
		  AND COALESCE(sp.modification_counter,0) > COALESCE(sp.rows,0) * %f
		ORDER BY COALESCE(sp.modification_counter,0) DESC;
	`, quoteDb(databaseName), limit, modificationPct)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[MSSQL] FetchStaleStatistics error for %s/%s: %v", instanceName, databaseName, err)
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtStaleStats, 0, limit)
	for rows.Next() {
		var r PerformanceDebtStaleStats
		if err := rows.Scan(&r.TableName, &r.StatsName, &r.LastUpdated, &r.Rows, &r.ModificationCounter); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type PerformanceDebtFileGrowth struct {
	FileName        string
	SizeMB          float64
	Growth          int64
	IsPercentGrowth bool
}

func (c *MssqlRepository) FetchAutogrowthRisks(instanceName, databaseName string, limit int) ([]PerformanceDebtFileGrowth, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}

	q := fmt.Sprintf(`
		USE %s;
		SELECT TOP (%d)
			name,
			size*8/1024.0 AS size_mb,
			growth,
			is_percent_growth
		FROM sys.database_files
		ORDER BY size DESC;
	`, quoteDb(databaseName), limit)
	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[MSSQL] FetchAutogrowthRisks error for %s/%s: %v", instanceName, databaseName, err)
		return nil, err
	}
	defer rows.Close()

	out := make([]PerformanceDebtFileGrowth, 0, limit)
	for rows.Next() {
		var r PerformanceDebtFileGrowth
		if err := rows.Scan(&r.FileName, &r.SizeMB, &r.Growth, &r.IsPercentGrowth); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (c *MssqlRepository) FetchVLFCount(instanceName, databaseName string) (int64, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := fmt.Sprintf(`
		USE %s;
		SELECT COUNT(*) AS vlf_count
		FROM sys.dm_db_log_info(DB_ID());
	`, quoteDb(databaseName))
	var n int64
	if err := db.QueryRow(q).Scan(&n); err != nil {
		log.Printf("[MSSQL] FetchVLFCount error for %s/%s: %v", instanceName, databaseName, err)
		return 0, err
	}
	return n, nil
}

func (c *MssqlRepository) FetchLastFullBackupAgeHours(instanceName, databaseName string) (float64, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := `
		SELECT TOP 1 DATEDIFF(MINUTE, backup_finish_date, GETDATE()) / 60.0 AS age_hours
		FROM msdb.dbo.backupset
		WHERE database_name = @p1
		  AND type = 'D'
		ORDER BY backup_finish_date DESC;
	`
	var age sql.NullFloat64
	if err := db.QueryRow(q, databaseName).Scan(&age); err != nil {
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

func (c *MssqlRepository) FetchFailedAgentJobs24h(instanceName string, limit int) ([]map[string]interface{}, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 50
	}
	q := fmt.Sprintf(`
		SELECT TOP (%d)
			j.name AS job_name,
			msdb.dbo.agent_datetime(h.run_date, h.run_time) AS run_dt,
			h.run_status
		FROM msdb.dbo.sysjobs j
		JOIN msdb.dbo.sysjobhistory h
		  ON j.job_id = h.job_id
		WHERE h.step_id = 0
		  AND h.run_status = 0
		  AND msdb.dbo.agent_datetime(h.run_date, h.run_time) >= DATEADD(HOUR, -24, GETDATE())
		ORDER BY run_dt DESC;
	`, limit)
	rows, err := db.Query(q)
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

func (c *MssqlRepository) FetchDisabledAgentJobs(instanceName string, limit int) ([]string, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	if limit <= 0 {
		limit = 200
	}
	q := fmt.Sprintf(`SELECT TOP (%d) name FROM msdb.dbo.sysjobs WHERE enabled = 0 ORDER BY name`, limit)
	rows, err := db.Query(q)
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

func (c *MssqlRepository) FetchConfigValueInUse(instanceName string, name string) (int64, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return 0, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := `SELECT value_in_use FROM sys.configurations WHERE name = @p1`
	var v int64
	if err := db.QueryRow(q, name).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func quoteDb(dbName string) string {
	// Minimal quoting for USE <db>.
	// Disallow dangerous characters; caller supplies db from config anyway.
	clean := strings.ReplaceAll(dbName, "]", "")
	return "[" + clean + "]"
}

