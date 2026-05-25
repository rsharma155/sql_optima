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
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/domain"
)

func sortWorkloadTrendPoints(points []domain.SqlServerWorkloadTrendPoint) {
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})
}

func workloadMetricsFilterSQL(filter domain.WorkloadQueryFilter) string {
	return sqlServerQueryAnalysisScopeSQL(filter.ExcludeSystem, "", filter.MonitoringLogins)
}

// sqlServerWorkloadFingerprintSelectSQL rolls up metrics_v2 rows by statement fingerprint (Workload top queries).
func sqlServerWorkloadFingerprintSelectSQL(col string) string {
	return sqlServerQueryAnalysisFingerprintSelectSQL(col) + `,
		       SUM(` + col + `total_rows)::bigint AS total_rows`
}

func workloadUsesDatabaseScope(filter domain.WorkloadQueryFilter) bool {
	return filter.Database != "" && filter.Database != "all"
}

func workloadDatabaseClause(argPos int) string {
	return fmt.Sprintf(" AND LOWER(TRIM(COALESCE(database_name, ''))) = LOWER(TRIM($%d))", argPos)
}

func (tl *TimescaleLogger) fillWorkloadDiagnostics(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter, s *domain.SqlServerWorkloadSummary) {
	var latestHist, latestMet, lastPoll *time.Time
	_ = tl.pool.QueryRow(ctx, `
		SELECT MAX(capture_timestamp) FROM sqlserver_query_stats_history
		WHERE server_id = $1`, serverID).Scan(&latestHist)
	_ = tl.pool.QueryRow(ctx, `
		SELECT MAX(capture_timestamp) FROM sqlserver_query_metrics_v2
		WHERE server_id = $1`, serverID).Scan(&latestMet)
	_ = tl.pool.QueryRow(ctx, `
		SELECT last_poll_time_utc FROM sqlserver_collector_instance_state
		WHERE server_id = $1`, serverID).Scan(&lastPoll)

	s.Diagnostics.LatestHistoryCapture = latestHist
	s.Diagnostics.LatestMetricsCapture = latestMet
	s.Diagnostics.CollectorLastPoll = lastPoll

	_ = tl.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sqlserver_query_stats_history
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3`,
		serverID, from, to).Scan(&s.Diagnostics.HistoryRowsInRange)

	_ = tl.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sqlserver_query_metrics_v2
		WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3`,
		serverID, from, to).Scan(&s.Diagnostics.MetricsRowsUnfiltered)

	if workloadUsesDatabaseScope(filter) {
		_ = tl.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM sqlserver_query_metrics_v2
			WHERE server_id = $1 AND capture_timestamp >= $2 AND capture_timestamp <= $3`+workloadDatabaseClause(4)+workloadMetricsFilterSQL(filter),
			serverID, from, to, filter.Database).Scan(&s.Diagnostics.MetricsRowsInRange)
	} else {
		s.Diagnostics.MetricsRowsInRange = s.Diagnostics.MetricsRowsUnfiltered
	}

	s.Diagnostics.DatabasesInRange, _ = tl.GetSqlServerDatabasesInRange(ctx, serverID, from, to, filter)
}

// GetSqlServerDatabasesInRange returns per-database row counts (optionally after workload read filters).
func (tl *TimescaleLogger) GetSqlServerDatabasesInRange(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadDatabaseActivity, error) {
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() || !from.Before(to) {
		from = to.Add(-1 * time.Hour)
	}
	readFilter := workloadMetricsFilterSQL(filter)
	rows, err := tl.pool.Query(ctx, `
		SELECT TRIM(COALESCE(database_name, 'unknown')) AS database_name,
		       COUNT(*)::bigint,
		       COALESCE(SUM(total_cpu_ms), 0)::bigint
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
	`+readFilter+`
		GROUP BY TRIM(COALESCE(database_name, 'unknown'))
		ORDER BY SUM(total_cpu_ms) DESC NULLS LAST`, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerDatabasesInRange: %w", err)
	}
	defer rows.Close()

	out := make([]domain.SqlServerWorkloadDatabaseActivity, 0)
	for rows.Next() {
		var d domain.SqlServerWorkloadDatabaseActivity
		if err := rows.Scan(&d.DatabaseName, &d.RowCount, &d.TotalCPUms); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetSqlServerPrimaryWorkloadDatabase picks the database with the most filtered CPU in range.
func (tl *TimescaleLogger) GetSqlServerPrimaryWorkloadDatabase(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (string, error) {
	dbs, err := tl.GetSqlServerDatabasesInRange(ctx, serverID, from, to, filter)
	if err != nil {
		return "", err
	}
	if db := pickWorkloadDatabaseName(dbs); db != "" {
		return db, nil
	}
	// Fallback: any database with raw rows (ignore read filters) so CRUD is not scoped to empty master.
	unfiltered := domain.WorkloadQueryFilter{
		ExcludeSystem:    false,
		MonitoringLogins: filter.MonitoringLogins,
	}
	raw, err := tl.GetSqlServerDatabasesInRange(ctx, serverID, from, to, unfiltered)
	if err != nil {
		return "", err
	}
	return pickWorkloadDatabaseName(raw), nil
}

func pickWorkloadDatabaseName(dbs []domain.SqlServerWorkloadDatabaseActivity) string {
	for _, d := range filterQueryAnalysisDatabases(dbs) {
		if d.RowCount > 0 && strings.TrimSpace(d.DatabaseName) != "" && !strings.EqualFold(d.DatabaseName, "unknown") {
			return d.DatabaseName
		}
	}
	filtered := filterQueryAnalysisDatabases(dbs)
	if len(filtered) > 0 && filtered[0].RowCount > 0 && strings.TrimSpace(filtered[0].DatabaseName) != "" {
		return filtered[0].DatabaseName
	}
	return ""
}

// GetSqlServerWorkloadSummary aggregates KPI data for the requested time range (TimescaleDB only).
func (tl *TimescaleLogger) GetSqlServerWorkloadSummary(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) (*domain.SqlServerWorkloadSummary, error) {
	var s domain.SqlServerWorkloadSummary
	var err error

	dbClause := ""
	args := []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = workloadDatabaseClause(4)
		args = append(args, filter.Database)
	}
	query := `
			SELECT
				COALESCE(SUM(total_cpu_ms), 0),
				COALESCE(SUM(total_executions), 0),
				COALESCE(SUM(total_logical_reads), 0),
				COALESCE(SUM(total_rows), 0),
				0
			FROM sqlserver_query_metrics_v2
			WHERE server_id = $1
			  AND capture_timestamp >= $2
			  AND capture_timestamp <= $3
		` + dbClause + workloadMetricsFilterSQL(filter)
	err = tl.pool.QueryRow(ctx, query, args...).Scan(
		&s.TotalCPUms, &s.TotalExecutions, &s.TotalLogicalReads, &s.TotalRows, &s.MaxMemoryGrantKB,
	)

	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadSummary: %w", err)
	}

	if s.TotalExecutions > 0 {
		s.AvgCPUPerExec = float64(s.TotalCPUms) / float64(s.TotalExecutions)
		s.AvgReadsPerExec = float64(s.TotalLogicalReads) / float64(s.TotalExecutions)
	}

	tl.fillWorkloadDiagnostics(ctx, serverID, from, to, filter, &s)
	return &s, nil
}

// GetSqlServerWorkloadTrends returns bucketed time-series data for workload visualization (TimescaleDB only).
func (tl *TimescaleLogger) GetSqlServerWorkloadTrends(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadTrendPoint, error) {
	duration := to.Sub(from)
	bucketSize := "1 minute"
	if duration > 12*time.Hour {
		bucketSize = "5 minutes"
	}
	if duration > 48*time.Hour {
		bucketSize = "15 minutes"
	}

	var query string
	var args []interface{}

	dbClause := ""
	args = []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = workloadDatabaseClause(4)
		args = append(args, filter.Database)
	}
	query = fmt.Sprintf(`
			SELECT
				time_bucket('%s', capture_timestamp) AS bucket,
				SUM(total_cpu_ms) AS cpu_ms,
				SUM(total_executions) AS execs,
				SUM(total_logical_reads) AS reads,
				SUM(total_rows) AS row_count,
				0 AS max_grant,
				0 AS max_dop,
				0 AS worst_query,
				SUM(total_cpu_ms) / NULLIF(SUM(total_executions), 0)::float AS avg_cpu,
				SUM(total_rows) / NULLIF(SUM(total_executions), 0)::float AS avg_rows
			FROM sqlserver_query_metrics_v2
			WHERE server_id = $1
			  AND capture_timestamp >= $2
			  AND capture_timestamp <= $3
		`, bucketSize) + dbClause + workloadMetricsFilterSQL(filter) + `
			GROUP BY bucket
			ORDER BY bucket ASC
		`

	rows, err := tl.pool.Query(ctx, query, args...)
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

// GetSqlServerWorkloadTrendsFromPerfCounters fills workload trend charts from instance-level
// perf counters when sqlserver_query_stats_history has no rows (e.g. query collector warming up).
func (tl *TimescaleLogger) GetSqlServerWorkloadTrendsFromPerfCounters(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]domain.SqlServerWorkloadTrendPoint, error) {
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
			time_bucket('%s', capture_timestamp) AS bucket,
			COALESCE(MAX(CASE WHEN counter_name = 'Batch Requests/sec' THEN value_per_sec END), 0) AS batch_rate,
			COALESCE(MAX(CASE WHEN counter_name = 'Page Reads/sec' THEN value_per_sec END), 0) AS page_reads_rate
		FROM sqlserver_perf_counters
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND counter_name IN ('Batch Requests/sec', 'Page Reads/sec')
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSize)

	rows, err := tl.pool.Query(ctx, query, serverID, from, to)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadTrendsFromPerfCounters: %w", err)
	}
	defer rows.Close()

	trends := make([]domain.SqlServerWorkloadTrendPoint, 0)
	for rows.Next() {
		var ts time.Time
		var batchRate, readsRate float64
		if err := rows.Scan(&ts, &batchRate, &readsRate); err != nil {
			continue
		}
		trends = append(trends, domain.SqlServerWorkloadTrendPoint{
			Timestamp:    ts,
			Executions:   int64(batchRate),
			LogicalReads: int64(readsRate),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// CPU trend: use sqlserver_health_kpis_v2 when query-history buckets are empty.
	cpuQ := fmt.Sprintf(`
		SELECT time_bucket('%s', capture_timestamp) AS bucket, AVG(sql_cpu_pct) AS sql_cpu_pct
		FROM sqlserver_health_kpis_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketSize)
	cpuRows, err := tl.pool.Query(ctx, cpuQ, serverID, from, to)
	if err != nil {
		return trends, nil
	}
	defer cpuRows.Close()

	cpuByBucket := make(map[int64]int64)
	for cpuRows.Next() {
		var ts time.Time
		var pct float64
		if err := cpuRows.Scan(&ts, &pct); err != nil {
			continue
		}
		// Chart expects CPU seconds per bucket; approximate from average % over 60s bucket.
		cpuByBucket[ts.Unix()] = int64(pct * 600)
	}
	if len(cpuByBucket) == 0 {
		return trends, nil
	}
	if len(trends) == 0 {
		for tsUnix, cpuMs := range cpuByBucket {
			trends = append(trends, domain.SqlServerWorkloadTrendPoint{
				Timestamp: time.Unix(tsUnix, 0).UTC(),
				CPUms:     cpuMs,
			})
		}
		sortWorkloadTrendPoints(trends)
		return trends, nil
	}
	for i := range trends {
		if cpu, ok := cpuByBucket[trends[i].Timestamp.Unix()]; ok {
			trends[i].CPUms = cpu
		}
	}
	return trends, nil
}

// GetSqlServerWorkloadTopOffenders identifies queries causing the most load in the given period.
// Reads from sqlserver_query_metrics_v2 which has enriched login/application data from plan enrichment.
func (tl *TimescaleLogger) GetSqlServerWorkloadTopOffenders(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadTopQuery, error) {
	if limit <= 0 {
		limit = 20
	}

	dbClause := ""
	args := []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = workloadDatabaseClause(4)
		args = append(args, filter.Database)
	}
	limitArg := len(args) + 1
	args = append(args, limit)

	dbScoped := workloadUsesDatabaseScope(filter)
	metricsFilter := workloadMetricsFilterSQL(filter)
	classFilter := sqlServerQueryAnalysisClassificationFilter(filter.ExcludeSystem, "q.")
	query := `
		SELECT` + sqlServerWorkloadFingerprintSelectSQL("q.") + `
		FROM sqlserver_query_metrics_v2 q
		WHERE q.server_id = $1
		  AND q.capture_timestamp >= $2
		  AND q.capture_timestamp <= $3
` + dbClause + metricsFilter + classFilter + sqlServerQueryAnalysisGroupByFingerprintSQL("q.", dbScoped) + `
		HAVING SUM(q.total_executions) > 0 OR SUM(q.total_cpu_ms) > 0
		ORDER BY SUM(q.total_cpu_ms) DESC
		LIMIT $` + fmt.Sprint(limitArg)

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("GetSqlServerWorkloadTopOffenders: %w", err)
	}
	defer rows.Close()

	offenders := make([]domain.SqlServerWorkloadTopQuery, 0)
	for rows.Next() {
		var q domain.SqlServerWorkloadTopQuery
		var qh int64
		var planCount, hashVariants int64
		var totalElapsed, totalReads int64
		var avgCPU, avgElapsed, avgReads float64
		err := rows.Scan(
			&q.StatementFingerprint, &qh, &hashVariants, &q.QueryText, &q.DatabaseName, &q.LoginName, &q.ProgramName, &planCount,
			&q.TotalExecutions, &q.TotalCPUms, &totalElapsed, &totalReads,
			&avgCPU, &avgElapsed, &avgReads, &q.LastSeen, &q.TotalRows,
		)
		if err != nil {
			slog.Error("[TSLogger] GetSqlServerWorkloadTopOffenders scan error", "err", err)
			continue
		}
		q.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		q.HashVariantCount = int(hashVariants)
		q.TotalReads = totalReads
		q.TotalElapsedMs = totalElapsed
		q.AvgCPUms = avgCPU
		q.AvgElapsedMs = avgElapsed
		_ = planCount
		offenders = append(offenders, q)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(offenders) > 0 {
		return offenders, nil
	}
	return tl.getSqlServerWorkloadTopOffendersFromHistory(ctx, serverID, from, to, limit, filter)
}

// getSqlServerWorkloadTopOffendersFromHistory is used when metrics_v2 rows are empty
// (e.g. session enrichment not yet linked) but query_stats_history has deltas.
func (tl *TimescaleLogger) getSqlServerWorkloadTopOffendersFromHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]domain.SqlServerWorkloadTopQuery, error) {
	dbClause := ""
	args := []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = ` AND EXISTS (
			SELECT 1 FROM sqlserver_query_metrics_v2 qm
			WHERE qm.server_id = qh.server_id AND qm.query_hash = qh.query_hash
			  AND LOWER(TRIM(COALESCE(qm.database_name, ''))) = LOWER(TRIM($4))
			LIMIT 1
		)`
		args = append(args, filter.Database)
	}
	limitArg := len(args) + 1
	args = append(args, limit)

	snapFilter := sqlServerSnapshotReadFilter(filter.ExcludeSystem, "s.")
	query := `
		SELECT
			qh.query_hash,
			COALESCE(MAX(s.statement_text), 'Plan Evicted') AS query_text,
			COALESCE(MAX(s.database_name), 'unknown') AS database_name,
			SUM(qh.cpu_delta_ms) AS total_cpu,
			SUM(qh.exec_delta) AS total_exec,
			SUM(qh.reads_delta) AS total_reads,
			SUM(qh.rows_delta) AS total_rows,
			MAX(s.last_execution_time) AS last_seen
		FROM sqlserver_query_stats_history qh
		INNER JOIN sqlserver_query_stats_snapshot_v2 s
			ON s.server_id = qh.server_id AND s.query_hash = qh.query_hash
		WHERE qh.server_id = $1
		  AND qh.capture_timestamp >= $2
		  AND qh.capture_timestamp <= $3
` + dbClause + snapFilter + `
		GROUP BY qh.query_hash
		HAVING SUM(qh.exec_delta) > 0 OR SUM(qh.cpu_delta_ms) > 0
		ORDER BY SUM(qh.cpu_delta_ms) DESC
		LIMIT $` + fmt.Sprint(limitArg)

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("getSqlServerWorkloadTopOffendersFromHistory: %w", err)
	}
	defer rows.Close()

	offenders := make([]domain.SqlServerWorkloadTopQuery, 0)
	for rows.Next() {
		var q domain.SqlServerWorkloadTopQuery
		var qh int64
		err := rows.Scan(
			&qh, &q.QueryText, &q.DatabaseName,
			&q.TotalCPUms, &q.TotalExecutions, &q.TotalReads, &q.TotalRows,
			&q.LastSeen,
		)
		if err != nil {
			continue
		}
		q.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		q.LoginName = "unknown"
		q.ProgramName = "unknown"
		if q.TotalExecutions > 0 {
			q.AvgCPUms = float64(q.TotalCPUms) / float64(q.TotalExecutions)
		}
		offenders = append(offenders, q)
	}
	return offenders, rows.Err()
}

// GetSqlServerWorkloadAppLoadTimeline returns CPU load timeline grouped by application.
// Reads from sqlserver_query_metrics_v2 which has enriched application_name from plan enrichment.
func (tl *TimescaleLogger) GetSqlServerWorkloadAppLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error) {
	duration := to.Sub(from)
	bucketSize := "5 minutes"
	if duration > 24*time.Hour {
		bucketSize = "15 minutes"
	}

	dbClause := ""
	args := []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = workloadDatabaseClause(4)
		args = append(args, filter.Database)
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
	`, bucketSize) + dbClause + workloadMetricsFilterSQL(filter) + `
		GROUP BY bucket, app_name
		ORDER BY bucket ASC, cpu_ms DESC
	`

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]map[string]interface{}, 0)
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
func (tl *TimescaleLogger) GetSqlServerWorkloadLoginLoadTimeline(ctx context.Context, serverID uuid.UUID, from, to time.Time, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error) {
	duration := to.Sub(from)
	bucketSize := "5 minutes"
	if duration > 24*time.Hour {
		bucketSize = "15 minutes"
	}

	dbClause := ""
	args := []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = workloadDatabaseClause(4)
		args = append(args, filter.Database)
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
	`, bucketSize) + dbClause + workloadMetricsFilterSQL(filter) + `
		GROUP BY bucket, login_name
		ORDER BY bucket ASC, cpu_ms DESC
	`

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]map[string]interface{}, 0)
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
func (tl *TimescaleLogger) GetSqlServerWorkloadTopApps(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error) {
	dbClause := ""
	args := []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = workloadDatabaseClause(4)
		args = append(args, filter.Database)
	}
	limitArg := len(args) + 1
	args = append(args, limit)

	query := `
		SELECT
			COALESCE(application_name, 'unknown') AS app_name,
			SUM(total_cpu_ms) AS total_cpu_ms,
			SUM(total_executions) AS total_executions
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
` + dbClause + workloadMetricsFilterSQL(filter) + `
		GROUP BY app_name
		ORDER BY total_cpu_ms DESC
		LIMIT $` + fmt.Sprint(limitArg)

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]map[string]interface{}, 0)
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
func (tl *TimescaleLogger) GetSqlServerWorkloadTopLogins(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int, filter domain.WorkloadQueryFilter) ([]map[string]interface{}, error) {
	dbClause := ""
	args := []interface{}{serverID, from, to}
	if workloadUsesDatabaseScope(filter) {
		dbClause = workloadDatabaseClause(4)
		args = append(args, filter.Database)
	}
	limitArg := len(args) + 1
	args = append(args, limit)

	query := `
		SELECT
			COALESCE(login_name, 'unknown') AS login_name,
			SUM(total_cpu_ms) AS total_cpu_ms,
			SUM(total_executions) AS total_executions
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
` + dbClause + workloadMetricsFilterSQL(filter) + `
		GROUP BY login_name
		ORDER BY total_cpu_ms DESC
		LIMIT $` + fmt.Sprint(limitArg)

	rows, err := tl.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]map[string]interface{}, 0)
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
