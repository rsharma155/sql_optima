// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository methods for the SQL Server Query Analysis Dashboard —
//
//	regression detection, plan instability, and watched-query metrics
//	via Query Store DMVs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/rsharma155/sql_optima/internal/models"
)

// FetchQueryRegressions compares the last-24h Query Store averages to the previous 24h
// and returns queries whose duration/cpu/reads regressed ≥ 50%.
func (c *SqlServerRepository) FetchQueryRegressions(instanceName string) ([]models.SqlServerQueryRegression, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	dbNames, err := c.listUserDatabaseNamesForQueryStore(db)
	if err != nil || len(dbNames) == 0 {
		log.Printf("[SQLSERVER] FetchQueryRegressions: no QS databases for %s: %v", instanceName, err)
		return nil, nil
	}

	var all []models.SqlServerQueryRegression
	for _, dbn := range dbNames {
		rows, err := fetchRegressionsForDB(db, dbn)
		if err != nil {
			log.Printf("[SQLSERVER] FetchQueryRegressions(%s/%s): %v", instanceName, dbn, err)
			continue
		}
		all = append(all, rows...)
	}
	return all, nil
}

func fetchRegressionsForDB(db *sql.DB, dbName string) ([]models.SqlServerQueryRegression, error) {
	_ = sqlServerQuoteBracket(dbName)
	const query = `
		WITH recent AS (
			SELECT /* SQL_OPTIMA */  
				CONVERT(VARCHAR(40), q.query_hash, 1) AS query_hash,
				qt.query_sql_text AS query_text,
				AVG(rs.avg_duration / 1000.0) AS avg_duration_ms,
				AVG(rs.avg_cpu_time / 1000.0) AS avg_cpu_ms,
				AVG(rs.avg_logical_io_reads) AS avg_reads,
				MAX(CAST(rs.last_execution_time AS DATETIME)) AS last_execution_time,
				MAX(CAST(p.is_forced_plan AS INT)) AS plan_changed
			FROM sys.query_store_runtime_stats rs
			JOIN sys.query_store_plan p ON rs.plan_id = p.plan_id
			JOIN sys.query_store_query q ON p.query_id = q.query_id
			JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
			JOIN sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
			WHERE rsi.start_time >= DATEADD(hour, -24, GETUTCDATE())
			  AND q.is_internal_query = 0
			  AND qt.query_sql_text NOT LIKE '%%sys.query_store%%'
			  AND q.last_bind_duration > 0
			GROUP BY q.query_hash, qt.query_sql_text
		),
		previous AS (
			SELECT /* SQL_OPTIMA */  
				CONVERT(VARCHAR(40), q.query_hash, 1) AS query_hash,
				AVG(rs.avg_duration / 1000.0) AS avg_duration_ms,
				AVG(rs.avg_cpu_time / 1000.0) AS avg_cpu_ms,
				AVG(rs.avg_logical_io_reads) AS avg_reads
			FROM sys.query_store_runtime_stats rs
			JOIN sys.query_store_plan p ON rs.plan_id = p.plan_id
			JOIN sys.query_store_query q ON p.query_id = q.query_id
			JOIN sys.query_store_runtime_stats_interval rsi ON rs.runtime_stats_interval_id = rsi.runtime_stats_interval_id
			WHERE rsi.start_time >= DATEADD(hour, -48, GETUTCDATE())
			  AND rsi.start_time < DATEADD(hour, -24, GETUTCDATE())
			GROUP BY q.query_hash
		)
		SELECT /* SQL_OPTIMA */   TOP 50
			r.query_hash,
			r.query_text,
			CASE
				WHEN p.avg_duration_ms > 0 AND ((r.avg_duration_ms - p.avg_duration_ms) / p.avg_duration_ms) >= 0.5 THEN 'duration'
				WHEN p.avg_cpu_ms > 0 AND ((r.avg_cpu_ms - p.avg_cpu_ms) / p.avg_cpu_ms) >= 0.5 THEN 'cpu'
				WHEN p.avg_reads > 0 AND ((r.avg_reads - p.avg_reads) / p.avg_reads) >= 0.5 THEN 'reads'
			END AS regression_type,
			ISNULL(p.avg_duration_ms, 0) AS previous_avg,
			r.avg_duration_ms AS current_avg,
			CASE WHEN p.avg_duration_ms > 0 THEN ((r.avg_duration_ms - p.avg_duration_ms) / p.avg_duration_ms) * 100 ELSE 0 END AS percent_change,
			ISNULL(r.plan_changed, 0) AS plan_changed
		FROM recent r
		JOIN previous p ON r.query_hash = p.query_hash
		WHERE (
			(p.avg_duration_ms > 0 AND ((r.avg_duration_ms - p.avg_duration_ms) / p.avg_duration_ms) >= 0.5)
			OR (p.avg_cpu_ms > 0 AND ((r.avg_cpu_ms - p.avg_cpu_ms) / p.avg_cpu_ms) >= 0.5)
			OR (p.avg_reads > 0 AND ((r.avg_reads - p.avg_reads) / p.avg_reads) >= 0.5)
		)
		ORDER BY percent_change DESC`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("regression query on %s: %w", dbName, err)
	}
	defer rows.Close()

	var results []models.SqlServerQueryRegression
	for rows.Next() {
		var r models.SqlServerQueryRegression
		var regType sql.NullString
		var planChanged sql.NullBool
		if err := rows.Scan(&r.QueryHash, &r.QueryText, &regType,
			&r.PreviousAvg, &r.CurrentAvg, &r.PercentChange, &planChanged); err != nil {
			log.Printf("[SQLSERVER] FetchQueryRegressions scan: %v", err)
			continue
		}
		if !regType.Valid || regType.String == "" {
			continue
		}
		r.RegressionType = regType.String
		r.DatabaseName = dbName
		if planChanged.Valid {
			r.PlanChanged = planChanged.Bool
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// FetchPlanInstability returns queries with more than 3 execution plans in Query Store.
func (c *SqlServerRepository) FetchPlanInstability(instanceName string) ([]models.SqlServerPlanInstability, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	dbNames, err := c.listUserDatabaseNamesForQueryStore(db)
	if err != nil || len(dbNames) == 0 {
		log.Printf("[SQLSERVER] FetchPlanInstability: no QS databases for %s: %v", instanceName, err)
		return nil, nil
	}

	var all []models.SqlServerPlanInstability
	for _, dbn := range dbNames {
		rows, err := fetchPlanInstabilityForDB(db, dbn)
		if err != nil {
			log.Printf("[SQLSERVER] FetchPlanInstability(%s/%s): %v", instanceName, dbn, err)
			continue
		}
		all = append(all, rows...)
	}
	return all, nil
}

func fetchPlanInstabilityForDB(db *sql.DB, dbName string) ([]models.SqlServerPlanInstability, error) {
	qb := sqlServerQuoteBracket(dbName)
	query := fmt.Sprintf(`
		USE %s;
		SELECT /* SQL_OPTIMA */   TOP 50
			CONVERT(VARCHAR(40), q.query_hash, 1) AS query_hash,
			qt.query_sql_text AS query_text,
			COUNT(DISTINCT p.plan_id) AS plan_count,
			MAX(rs.last_execution_time) AS last_execution_time
		FROM sys.query_store_plan p
		JOIN sys.query_store_query q ON p.query_id = q.query_id
		JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		JOIN sys.query_store_runtime_stats rs ON rs.plan_id = p.plan_id
		WHERE q.is_internal_query = 0
		  AND qt.query_sql_text NOT LIKE '%%sys.query_store%%'
		  AND q.last_bind_duration > 0
		GROUP BY q.query_hash, qt.query_sql_text
		HAVING COUNT(DISTINCT p.plan_id) > 3
		ORDER BY plan_count DESC`, qb)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("plan instability query on %s: %w", dbName, err)
	}
	defer rows.Close()

	var results []models.SqlServerPlanInstability
	for rows.Next() {
		var r models.SqlServerPlanInstability
		var lastExec sql.NullTime
		if err := rows.Scan(&r.QueryHash, &r.QueryText, &r.PlanCount, &lastExec); err != nil {
			log.Printf("[SQLSERVER] FetchPlanInstability scan: %v", err)
			continue
		}
		r.DatabaseName = dbName
		if lastExec.Valid {
			r.LastExecutionTime = lastExec.Time
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// FetchWatchedQueryStats fetches current Query Store statistics for a specific query hash.
// Used for populating watched-query snapshot data.
func (c *SqlServerRepository) FetchWatchedQueryStats(ctx context.Context, instanceName, dbName, queryHash string) (*models.SqlServerWatchedQuerySnapshot, error) {
	if dbName == "" {
		return nil, fmt.Errorf("FetchWatchedQueryStats: database name is required")
	}
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	qb := sqlServerQuoteBracket(dbName)
	query := fmt.Sprintf(`
		USE %s;
		SELECT /* SQL_OPTIMA */   TOP 1
			ISNULL(rs.count_executions, 0) AS executions,
			ISNULL(rs.avg_duration / 1000.0, 0) AS avg_duration_ms,
			ISNULL(rs.avg_cpu_time / 1000.0, 0) AS avg_cpu_ms,
			ISNULL(rs.avg_logical_io_reads, 0) AS avg_reads,
			ISNULL(rs.avg_duration * rs.count_executions / 1000.0, 0) AS total_duration_ms,
			ISNULL(rs.avg_cpu_time * rs.count_executions / 1000.0, 0) AS total_cpu_ms,
			(SELECT /* SQL_OPTIMA */   COUNT(DISTINCT p2.plan_id) FROM sys.query_store_plan p2 WHERE p2.query_id = q.query_id) AS plan_count,
			rs.last_execution_time,
			CAST(p.query_plan AS NVARCHAR(MAX)) AS query_plan,
			qt.query_sql_text
		FROM sys.query_store_runtime_stats rs
		JOIN sys.query_store_plan p ON rs.plan_id = p.plan_id
		JOIN sys.query_store_query q ON p.query_id = q.query_id
		JOIN sys.query_store_query_text qt ON q.query_text_id = qt.query_text_id
		WHERE CONVERT(VARCHAR(40), q.query_hash, 1) COLLATE DATABASE_DEFAULT = @p1 COLLATE DATABASE_DEFAULT
		  AND q.is_internal_query = 0
		ORDER BY rs.last_execution_time DESC`, qb)

	var s models.SqlServerWatchedQuerySnapshot
	var lastExec sql.NullTime
	var plan, text sql.NullString
	err := db.QueryRowContext(ctx, query, queryHash).Scan(
		&s.Executions, &s.AvgDurationMs, &s.AvgCpuMs, &s.AvgReads,
		&s.TotalDurationMs, &s.TotalCpuMs, &s.PlanCount, &lastExec,
		&plan, &text)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("FetchWatchedQueryStats: %w", err)
	}
	if lastExec.Valid {
		s.LastExecutionTime = lastExec.Time
	}
	if plan.Valid {
		s.QueryPlan = plan.String
	}
	if text.Valid {
		// Store text in a temporary field or reuse existing models
		s.QueryText = text.String
	}
	return &s, nil
}

// FetchQueryPlans returns plan metadata from Query Store for a specific query hash.
func (c *SqlServerRepository) FetchQueryPlans(ctx context.Context, instanceName, dbName, queryHash string) ([]models.SqlServerQueryPlanInfo, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	qb := sqlServerQuoteBracket(dbName)
	query := fmt.Sprintf(`
		USE %s;
		SELECT /* SQL_OPTIMA */  
			p.plan_id,
			ISNULL(AVG(rs.avg_duration / 1000.0), 0) AS avg_duration_ms,
			ISNULL(AVG(rs.avg_cpu_time / 1000.0), 0) AS avg_cpu_ms,
			ISNULL(AVG(rs.avg_logical_io_reads), 0) AS avg_reads,
			ISNULL(SUM(rs.count_executions), 0) AS executions,
			p.is_forced_plan,
			MAX(rs.last_execution_time) AS last_execution_time,
			p.initial_compile_start_time AS created_at,
			CAST(p.query_plan AS NVARCHAR(MAX)) AS query_plan
		FROM sys.query_store_plan p
		JOIN sys.query_store_query q ON p.query_id = q.query_id
		JOIN sys.query_store_runtime_stats rs ON rs.plan_id = p.plan_id
		WHERE CONVERT(VARCHAR(40), q.query_hash, 1) COLLATE DATABASE_DEFAULT = @p1 COLLATE DATABASE_DEFAULT
		GROUP BY p.plan_id, p.is_forced_plan, p.initial_compile_start_time, CAST(p.query_plan AS NVARCHAR(MAX))
		ORDER BY avg_duration_ms DESC`, qb)

	rows, err := db.QueryContext(ctx, query, queryHash)
	if err != nil {
		return nil, fmt.Errorf("FetchQueryPlans: %w", err)
	}
	defer rows.Close()

	var results []models.SqlServerQueryPlanInfo
	for rows.Next() {
		var p models.SqlServerQueryPlanInfo
		var lastExec, created sql.NullTime
		if err := rows.Scan(&p.PlanID, &p.AvgDurationMs, &p.AvgCpuMs, &p.AvgReads,
			&p.Executions, &p.IsForcedPlan, &lastExec, &created, &p.QueryPlan); err != nil {
			log.Printf("[SQLSERVER] FetchQueryPlans scan: %v", err)
			continue
		}
		if lastExec.Valid {
			p.LastExecutionTime = lastExec.Time
		}
		if created.Valid {
			p.CreatedAt = created.Time
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// FetchQueryWaitStats returns wait statistics from Query Store for a specific query hash.
func (c *SqlServerRepository) FetchQueryWaitStats(ctx context.Context, instanceName, dbName, queryHash string) ([]models.SqlServerQueryWaitStat, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	qb := sqlServerQuoteBracket(dbName)
	query := fmt.Sprintf(`
		USE %s;
		SELECT /* SQL_OPTIMA */  
			ws.wait_category_desc,
			AVG(ws.avg_query_wait_time_ms) AS avg_wait_ms,
			SUM(ws.total_query_wait_time_ms) AS total_wait_ms
		FROM sys.query_store_wait_stats ws
		JOIN sys.query_store_plan p ON ws.plan_id = p.plan_id
		JOIN sys.query_store_query q ON p.query_id = q.query_id
		WHERE CONVERT(VARCHAR(40), q.query_hash, 1) COLLATE DATABASE_DEFAULT = @p1 COLLATE DATABASE_DEFAULT
		GROUP BY ws.wait_category_desc
		ORDER BY total_wait_ms DESC`, qb)

	rows, err := db.QueryContext(ctx, query, queryHash)
	if err != nil {
		// wait_stats may not exist on older SQL Server versions
		log.Printf("[SQLSERVER] FetchQueryWaitStats(%s): %v — may not be supported", instanceName, err)
		return nil, nil
	}
	defer rows.Close()

	var results []models.SqlServerQueryWaitStat
	for rows.Next() {
		var w models.SqlServerQueryWaitStat
		if err := rows.Scan(&w.WaitCategory, &w.AvgWaitMs, &w.TotalWaitMs); err != nil {
			log.Printf("[SQLSERVER] FetchQueryWaitStats scan: %v", err)
			continue
		}
		results = append(results, w)
	}
	return results, rows.Err()
}
