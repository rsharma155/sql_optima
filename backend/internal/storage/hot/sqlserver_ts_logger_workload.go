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
	"log/slog"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain"
)

// GetSqlServerWorkloadSummary aggregates KPI data for the requested time range.
func (tl *TimescaleLogger) GetSqlServerWorkloadSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time) (*domain.SqlServerWorkloadSummary, error) {
	query := `
		SELECT
			COALESCE(SUM(cpu_delta_ms), 0),
			COALESCE(SUM(exec_delta), 0),
			COALESCE(SUM(reads_delta), 0),
			COALESCE(SUM(rows_delta), 0),
			COALESCE(MAX(period_max_grant_kb), 0)
		FROM sqlserver_query_stats_history
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
	`

	var s domain.SqlServerWorkloadSummary
	err := tl.pool.QueryRow(ctx, query, serverID, from, to).Scan(
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
func (tl *TimescaleLogger) GetSqlServerWorkloadTrends(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.SqlServerWorkloadTrendPoint, error) {
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
			time_bucket('%s', qh.capture_timestamp) AS bucket,
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
		WHERE qh.server_id = $1
		  AND qh.capture_timestamp >= $2
		  AND qh.capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSize)

	rows, err := tl.pool.Query(ctx, query, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadTrends: %w", err)
	}
	defer rows.Close()

	trends := make([]domain.SqlServerWorkloadTrendPoint, 0)
	for rows.Next() {
		var p domain.SqlServerWorkloadTrendPoint
		var avgCPU, avgRows *float64
		err := rows.Scan(
			&p.Timestamp, &p.CPUms, &p.Executions, &p.LogicalReads, &p.RowsProcessed,
			&p.MaxGrantKB, &p.MaxDOP, &p.WorstQueryms, &avgCPU, &avgRows,
		)
		if err != nil {
			slog.Error("[TSLogger] GetSqlServerWorkloadTrends scan error", "err", err)
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
// Reads from sqlserver_query_metrics_v2 which has enriched login/application data from plan enrichment.
func (tl *TimescaleLogger) GetSqlServerWorkloadTopOffenders(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]domain.SqlServerWorkloadTopQuery, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT
			query_hash,
			COALESCE(MAX(statement_text), 'Plan Evicted') AS query_text,
			COALESCE(MAX(database_name), 'unknown') AS database_name,
			COALESCE(MAX(login_name), 'unknown') AS login_name,
			COALESCE(MAX(application_name), 'unknown') AS program_name,
			SUM(total_cpu_ms) AS total_cpu,
			SUM(total_executions) AS total_exec,
			SUM(total_logical_reads) AS total_reads,
			SUM(total_rows) AS total_rows,
			MAX(last_execution_time) AS last_seen
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND is_user_workload = 1
		  AND COALESCE(query_text_raw, '') NOT LIKE '/* SQL_OPTIMA */%'
		  AND COALESCE(query_text_raw, '') NOT LIKE '%sys.dm_%'
		  AND COALESCE(query_text_raw, '') NOT LIKE '%sys.partitions%'
		  AND COALESCE(query_text_raw, '') NOT LIKE '%sys.plan_%'
		  AND COALESCE(query_text_raw, '') NOT LIKE '%backup%'
		  AND COALESCE(query_text_raw, '') NOT LIKE '%restore%'
		  AND COALESCE(query_text_raw, '') NOT LIKE '%is_ms_shipped%'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE 'FETCH NEXT FROM %'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE 'SET %'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE 'DECLARE %'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE '(@%'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE 'CREATE %'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE 'ALTER %'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE 'CHECKPOINT%'
		  AND UPPER(COALESCE(query_text_raw, '')) NOT LIKE 'DBCC %'
		  AND COALESCE(login_name, '') <> 'dbmonitor_user'
		  AND COALESCE(application_name, '') NOT IN ('sql-optima', 'SQLServerMS', 'SQL Server Profiler', 'SQLAgent - TSQL JobStep')
		  AND COALESCE(application_name, '') NOT LIKE 'Microsoft SQL Server Management Studio%'
		  AND COALESCE(application_name, '') NOT LIKE 'SQLAgent%'
		GROUP BY query_hash
		ORDER BY SUM(total_cpu_ms) DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, serverID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadTopOffenders: %w", err)
	}
	defer rows.Close()

	offenders := make([]domain.SqlServerWorkloadTopQuery, 0)
	for rows.Next() {
		var q domain.SqlServerWorkloadTopQuery
		var qh int64
		err := rows.Scan(
			&qh, &q.QueryText, &q.DatabaseName, &q.LoginName, &q.ProgramName,
			&q.TotalCPUms, &q.TotalExecutions, &q.TotalReads, &q.TotalRows,
			&q.LastSeen,
		)
		if err != nil {
			slog.Error("[TSLogger] GetSqlServerWorkloadTopOffenders scan error", "err", err)
			continue
		}
		q.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		if q.TotalExecutions > 0 {
			q.AvgCPUms = float64(q.TotalCPUms) / float64(q.TotalExecutions)
		}
		offenders = append(offenders, q)
	}

	return offenders, rows.Err()
}

// GetSqlServerWorkloadAppLoadTimeline returns CPU load timeline grouped by application.
// Reads from sqlserver_query_metrics_v2 which has enriched application_name from plan enrichment.
func (tl *TimescaleLogger) GetSqlServerWorkloadAppLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	duration := to.Sub(from)
	bucketSize := "5 minutes"
	if duration > 24*time.Hour {
		bucketSize = "15 minutes"
	}

	query := fmt.Sprintf(`
		SELECT
			time_bucket('%s', capture_timestamp) AS bucket,
			COALESCE(application_name, 'unknown') AS app_name,
			SUM(total_cpu_ms) AS cpu_ms
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND is_user_workload = 1
		GROUP BY bucket, app_name
		ORDER BY bucket ASC, cpu_ms DESC
	`, bucketSize)

	rows, err := tl.pool.Query(ctx, query, serverID, from, to)
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
// Reads from sqlserver_query_metrics_v2 which has enriched login_name from plan enrichment.
func (tl *TimescaleLogger) GetSqlServerWorkloadLoginLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	duration := to.Sub(from)
	bucketSize := "5 minutes"
	if duration > 24*time.Hour {
		bucketSize = "15 minutes"
	}

	query := fmt.Sprintf(`
		SELECT
			time_bucket('%s', capture_timestamp) AS bucket,
			COALESCE(login_name, 'unknown') AS login_name,
			SUM(total_cpu_ms) AS cpu_ms
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND is_user_workload = 1
		GROUP BY bucket, login_name
		ORDER BY bucket ASC, cpu_ms DESC
	`, bucketSize)

	rows, err := tl.pool.Query(ctx, query, serverID, from, to)
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
// Reads from sqlserver_query_metrics_v2 which has enriched application_name from plan enrichment.
func (tl *TimescaleLogger) GetSqlServerWorkloadTopApps(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			COALESCE(application_name, 'unknown') AS app_name,
			SUM(total_cpu_ms) AS total_cpu_ms,
			SUM(total_executions) AS total_executions
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND is_user_workload = 1
		GROUP BY app_name
		ORDER BY total_cpu_ms DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, serverID, from, to, limit)
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
// Reads from sqlserver_query_metrics_v2 which has enriched login_name from plan enrichment.
func (tl *TimescaleLogger) GetSqlServerWorkloadTopLogins(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			COALESCE(login_name, 'unknown') AS login_name,
			SUM(total_cpu_ms) AS total_cpu_ms,
			SUM(total_executions) AS total_executions
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND is_user_workload = 1
		GROUP BY login_name
		ORDER BY total_cpu_ms DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, serverID, from, to, limit)
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
