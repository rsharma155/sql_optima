// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage-layer for SQL Server Workload Observability Dashboard.
//          Provides high-performance aggregation from the sqlserver_query_stats_history hypertable.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/domain"
)

// GetSqlServerWorkloadSummary aggregates KPI data for the requested time range.
func (tl *TimescaleLogger) GetSqlServerWorkloadSummary(ctx context.Context, instanceID string, from, to time.Time) (*domain.SqlServerWorkloadSummary, error) {
	query := `
		SELECT
			COALESCE(SUM(qh.cpu_delta_ms), 0),
			COALESCE(SUM(qh.exec_delta), 0),
			COALESCE(SUM(qh.reads_delta), 0),
			COALESCE(SUM(qh.rows_delta), 0),
			COALESCE(MAX(qh.period_max_grant_kb), 0)
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = qh.instance_id
		 AND class.query_hash = qh.query_hash
		WHERE qh.instance_id = $1
		  AND qh.ts >= $2
		  AND qh.ts <= $3
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
	`

	var s domain.SqlServerWorkloadSummary
	err := tl.pool.QueryRow(ctx, query, instanceID, from, to).Scan(
		&s.TotalCPUms, &s.TotalExecutions, &s.TotalLogicalReads, &s.TotalRows, &s.MaxMemoryGrantKB,
	)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadSummary: %w", err)
	}

	if s.TotalExecutions > 0 {
		s.AvgCPUPerExec = float64(s.TotalCPUms) / float64(s.TotalExecutions)
		s.AvgReadsPerExec = float64(s.TotalLogicalReads) / float64(s.TotalExecutions)
	}

	return &s, nil
}

// GetSqlServerWorkloadTrends returns bucketed time-series data for workload visualization.
func (tl *TimescaleLogger) GetSqlServerWorkloadTrends(ctx context.Context, instanceID string, from, to time.Time) ([]domain.SqlServerWorkloadTrendPoint, error) {
	// Dynamically determine bucket size based on range
	duration := to.Sub(from)
	bucketSize := "1 minute"
	if duration > 12*time.Hour {
		bucketSize = "5 minutes"
	}
	if duration > 48*time.Hour {
		bucketSize = "15 minutes"
	}

	query := fmt.Sprintf(`
		SELECT
			time_bucket('%s', qh.ts) AS bucket,
			SUM(qh.cpu_delta_ms) AS cpu_ms,
			SUM(qh.exec_delta) AS execs,
			SUM(qh.reads_delta) AS reads,
			SUM(qh.rows_delta) AS rows,
			MAX(qh.period_max_grant_kb) AS max_grant,
			MAX(qh.period_max_dop) AS max_dop,
			MAX(qh.period_max_cpu_ms) AS worst_query,
			SUM(qh.cpu_delta_ms) / NULLIF(SUM(qh.exec_delta), 0)::float AS avg_cpu,
			SUM(qh.rows_delta) / NULLIF(SUM(qh.exec_delta), 0)::float AS avg_rows
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = qh.instance_id
		 AND class.query_hash = qh.query_hash
		WHERE qh.instance_id = $1
		  AND qh.ts >= $2
		  AND qh.ts <= $3
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSize)

	rows, err := tl.pool.Query(ctx, query, instanceID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadTrends: %w", err)
	}
	defer rows.Close()

	var trends []domain.SqlServerWorkloadTrendPoint
	for rows.Next() {
		var p domain.SqlServerWorkloadTrendPoint
		var avgCPU, avgRows *float64
		err := rows.Scan(
			&p.Timestamp, &p.CPUms, &p.Executions, &p.LogicalReads, &p.RowsProcessed,
			&p.MaxGrantKB, &p.MaxDOP, &p.WorstQueryms, &avgCPU, &avgRows,
		)
		if err != nil {
			log.Printf("[TSLogger] GetSqlServerWorkloadTrends scan error: %v", err)
			continue
		}
		if avgCPU != nil {
			p.AvgCPUms = *avgCPU
		}
		if avgRows != nil {
			p.AvgRows = *avgRows
		}
		trends = append(trends, p)
	}

	return trends, rows.Err()
}

// GetSqlServerWorkloadTopOffenders identifies queries causing the most load in the given period.
func (tl *TimescaleLogger) GetSqlServerWorkloadTopOffenders(ctx context.Context, instanceID string, from, to time.Time, limit int) ([]domain.SqlServerWorkloadTopQuery, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		WITH history_agg AS (
			SELECT
				qh.query_hash,
				SUM(qh.cpu_delta_ms) AS total_cpu,
				SUM(qh.exec_delta) AS total_exec,
				SUM(qh.reads_delta) AS total_reads,
				SUM(qh.rows_delta) AS total_rows,
				MAX(qh.ts) AS last_seen
			FROM sqlserver_query_stats_history qh
			LEFT JOIN sqlserver_query_classification_dim class
			  ON class.instance_id = qh.instance_id
			 AND class.query_hash = qh.query_hash
			WHERE qh.instance_id = $1
			  AND qh.ts >= $2
			  AND qh.ts <= $3
			  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
			GROUP BY qh.query_hash
		)
		SELECT
			encode(h.query_hash, 'hex'),
			COALESCE(MAX(s.statement_text), 'Plan Evicted'),
			COALESCE(MAX(s.database_name), 'unknown'),
			COALESCE(MAX(dim.login_name), 'unknown'),
			COALESCE(MAX(dim.program_name), 'unknown'),
			h.total_cpu,
			h.total_exec,
			h.total_reads,
			h.total_rows,
			h.last_seen
		FROM history_agg h
		LEFT JOIN sqlserver_query_stats_snapshot_v2 s
		  ON s.instance_id = $1
		 AND s.query_hash = h.query_hash
		LEFT JOIN sqlserver_query_identity_dim dim
		  ON dim.instance_id = $1
		 AND dim.query_hash = h.query_hash
		GROUP BY h.query_hash, h.total_cpu, h.total_exec, h.total_reads, h.total_rows, h.last_seen
		ORDER BY h.total_cpu DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, instanceID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadTopOffenders: %w", err)
	}
	defer rows.Close()

	var offenders []domain.SqlServerWorkloadTopQuery
	for rows.Next() {
		var q domain.SqlServerWorkloadTopQuery
		err := rows.Scan(
			&q.QueryHash, &q.QueryText, &q.DatabaseName, &q.LoginName, &q.ProgramName,
			&q.TotalCPUms, &q.TotalExecutions, &q.TotalReads, &q.TotalRows,
			&q.LastSeen,
		)
		if err != nil {
			log.Printf("[TSLogger] GetSqlServerWorkloadTopOffenders scan error: %v", err)
			continue
		}
		q.QueryHash = "0x" + q.QueryHash
		if q.TotalExecutions > 0 {
			q.AvgCPUms = float64(q.TotalCPUms) / float64(q.TotalExecutions)
		}
		offenders = append(offenders, q)
	}

	return offenders, rows.Err()
}

// GetSqlServerWorkloadAppLoadTimeline returns CPU load timeline grouped by application.
func (tl *TimescaleLogger) GetSqlServerWorkloadAppLoadTimeline(ctx context.Context, instanceID string, from, to time.Time) ([]map[string]interface{}, error) {
	duration := to.Sub(from)
	bucketSize := "5 minutes"
	if duration > 24*time.Hour {
		bucketSize = "15 minutes"
	}

	query := fmt.Sprintf(`
		SELECT
			time_bucket('%s', qh.ts) AS bucket,
			COALESCE(dim.program_name, 'unknown') AS app_name,
			SUM(qh.cpu_delta_ms) AS cpu_ms
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_identity_dim dim
		  ON dim.instance_id = qh.instance_id
		 AND dim.query_hash = qh.query_hash
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = qh.instance_id
		 AND class.query_hash = qh.query_hash
		WHERE qh.instance_id = $1
		  AND qh.ts >= $2
		  AND qh.ts <= $3
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		GROUP BY bucket, app_name
		ORDER BY bucket ASC, cpu_ms DESC
	`, bucketSize)

	rows, err := tl.pool.Query(ctx, query, instanceID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var appName string
		var cpuMS int64
		if err := rows.Scan(&bucket, &appName, &cpuMS); err == nil {
			results = append(results, map[string]interface{}{
				"bucket":   bucket,
				"app_name": appName,
				"cpu_ms":   cpuMS,
			})
		}
	}
	return results, rows.Err()
}

// GetSqlServerWorkloadLoginLoadTimeline returns CPU load timeline grouped by login.
func (tl *TimescaleLogger) GetSqlServerWorkloadLoginLoadTimeline(ctx context.Context, instanceID string, from, to time.Time) ([]map[string]interface{}, error) {
	duration := to.Sub(from)
	bucketSize := "5 minutes"
	if duration > 24*time.Hour {
		bucketSize = "15 minutes"
	}

	query := fmt.Sprintf(`
		SELECT
			time_bucket('%s', qh.ts) AS bucket,
			COALESCE(dim.login_name, 'unknown') AS login_name,
			SUM(qh.cpu_delta_ms) AS cpu_ms
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_identity_dim dim
		  ON dim.instance_id = qh.instance_id
		 AND dim.query_hash = qh.query_hash
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = qh.instance_id
		 AND class.query_hash = qh.query_hash
		WHERE qh.instance_id = $1
		  AND qh.ts >= $2
		  AND qh.ts <= $3
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		GROUP BY bucket, login_name
		ORDER BY bucket ASC, cpu_ms DESC
	`, bucketSize)

	rows, err := tl.pool.Query(ctx, query, instanceID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var loginName string
		var cpuMS int64
		if err := rows.Scan(&bucket, &loginName, &cpuMS); err == nil {
			results = append(results, map[string]interface{}{
				"bucket":     bucket,
				"login_name": loginName,
				"cpu_ms":     cpuMS,
			})
		}
	}
	return results, rows.Err()
}

// GetSqlServerWorkloadTopApps returns top applications by CPU consumption.
func (tl *TimescaleLogger) GetSqlServerWorkloadTopApps(ctx context.Context, instanceID string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			COALESCE(dim.program_name, 'unknown') AS app_name,
			SUM(qh.cpu_delta_ms) AS total_cpu_ms,
			SUM(qh.exec_delta) AS total_executions
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_identity_dim dim
		  ON dim.instance_id = qh.instance_id
		 AND dim.query_hash = qh.query_hash
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = qh.instance_id
		 AND class.query_hash = qh.query_hash
		WHERE qh.instance_id = $1
		  AND qh.ts >= $2
		  AND qh.ts <= $3
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		GROUP BY app_name
		ORDER BY total_cpu_ms DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, instanceID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var appName string
		var cpuMS, execs int64
		if err := rows.Scan(&appName, &cpuMS, &execs); err == nil {
			results = append(results, map[string]interface{}{
				"app_name":         appName,
				"total_cpu_ms":     cpuMS,
				"total_executions": execs,
			})
		}
	}
	return results, rows.Err()
}

// GetSqlServerWorkloadTopLogins returns top logins by CPU consumption.
func (tl *TimescaleLogger) GetSqlServerWorkloadTopLogins(ctx context.Context, instanceID string, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			COALESCE(dim.login_name, 'unknown') AS login_name,
			SUM(qh.cpu_delta_ms) AS total_cpu_ms,
			SUM(qh.exec_delta) AS total_executions
		FROM sqlserver_query_stats_history qh
		LEFT JOIN sqlserver_query_identity_dim dim
		  ON dim.instance_id = qh.instance_id
		 AND dim.query_hash = qh.query_hash
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = qh.instance_id
		 AND class.query_hash = qh.query_hash
		WHERE qh.instance_id = $1
		  AND qh.ts >= $2
		  AND qh.ts <= $3
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		GROUP BY login_name
		ORDER BY total_cpu_ms DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, instanceID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var loginName string
		var cpuMS, execs int64
		if err := rows.Scan(&loginName, &cpuMS, &execs); err == nil {
			results = append(results, map[string]interface{}{
				"login_name":       loginName,
				"total_cpu_ms":     cpuMS,
				"total_executions": execs,
			})
		}
	}
	return results, rows.Err()
}
