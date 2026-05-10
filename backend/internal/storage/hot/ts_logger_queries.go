// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage-layer methods for SQL Server query performance metrics.
//          Handles aggregation and retrieval from the sqlserver_query_metrics_v2 hypertable.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) GetSQLServerTopQueries(ctx context.Context, instanceName string, limit int, database string) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT MAX(q.bucket_end) as capture_timestamp, q.query_hash, MAX(q.query_text) as query_text, SUM(q.delta_executions) as execution_count,
		       SUM(q.delta_cpu_ms) as cpu_time_ms, SUM(q.delta_duration_ms) as exec_time_ms, SUM(q.delta_logical_reads) as logical_reads,
		       q.database_name, '' as login_name, '' as program_name
		FROM monitor.sqlserver_query_store_interval q
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = q.server_instance_name
		 AND class.query_hash = q.query_hash
		WHERE UPPER(q.server_instance_name) = UPPER($1)
		  AND q.bucket_end >= NOW() - INTERVAL '1 hour'
		  AND ($3 = '' OR q.database_name = $3)
		  AND q.query_text NOT LIKE '%/* SQL_OPTIMA */%'
		  AND (q.delta_cpu_ms > 20 OR q.delta_duration_ms > 20)
		  AND UPPER(q.query_text) NOT LIKE 'FETCH NEXT FROM %'
		  AND UPPER(q.query_text) NOT LIKE 'SET %'
		  AND UPPER(q.query_text) NOT LIKE 'DECLARE %'
		  AND UPPER(q.query_text) NOT LIKE '(@%'
		  AND UPPER(q.query_text) NOT LIKE 'CREATE %'
		  AND UPPER(q.query_text) NOT LIKE 'ALTER %'
		  AND UPPER(q.query_text) NOT LIKE 'CHECKPOINT%'
		  AND UPPER(q.query_text) NOT LIKE 'DBCC %'
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		GROUP BY q.query_hash, q.database_name
		ORDER BY SUM(q.delta_cpu_ms) DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit, strings.TrimSpace(database))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var qh int64
		var queryText sql.NullString
		var dbName, loginName, programName sql.NullString
		var execCount int64
		var cpuTimeMs, execTimeMs, logicalReads float64

		if err := rows.Scan(&ts, &qh, &queryText, &execCount, &cpuTimeMs, &execTimeMs, &logicalReads,
			&dbName, &loginName, &programName); err != nil {
			log.Printf("[TSLogger] GetSQLServerTopQueries scan: %v", err)
			continue
		}

		var avgCpu, avgReads float64
		if execCount > 0 {
			avgCpu = cpuTimeMs / float64(execCount)
			avgReads = logicalReads / float64(execCount)
		}

		results = append(results, map[string]interface{}{
			"capture_timestamp":   ts,
			"timestamp":           ts,
			"query_hash":          fmt.Sprintf("0x%X", uint64(qh)),
			"query_text":          queryText.String,
			"execution_count":     execCount,
			"avg_cpu_ms":          avgCpu,
			"total_cpu_ms":        cpuTimeMs,
			"total_cpu_time_ms":   cpuTimeMs,
			"exec_time_ms":        execTimeMs,
			"total_exec_time_ms":  execTimeMs,
			"avg_logical_reads":   avgReads,
			"total_logical_reads": logicalReads,
			"database_name":       dbName.String,
			"login_name":          loginName.String,
			"program_name":        programName.String,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerTopQueriesWithRange(ctx context.Context, instanceName string, limit int, from, to string, database string) ([]map[string]interface{}, error) {
	var start, end time.Time
	var err error
	if strings.TrimSpace(from) != "" && strings.TrimSpace(to) != "" {
		start, end, err = parseTimeRangeRFC3339(from, to)
		if err != nil {
			return nil, err
		}
	} else {
		start, end, err = parseTimeRange(from, to)
		if err != nil {
			return nil, err
		}
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT q.query_hash,
		       MAX(q.query_text_raw) AS query_text,
		       SUM(q.total_executions)::bigint AS total_executions,
		       SUM(q.total_cpu_ms)::bigint AS sum_cpu_ms,
		       CASE WHEN SUM(q.total_executions) > 0
		            THEN SUM(q.total_cpu_ms)::float8 / NULLIF(SUM(q.total_executions)::float8, 0)
		            ELSE 0 END AS avg_cpu_ms,
		       SUM(q.total_logical_reads)::bigint AS sum_logical_reads,
		       CASE WHEN SUM(q.total_executions) > 0
		            THEN SUM(q.total_logical_reads)::float8 / NULLIF(SUM(q.total_executions)::float8, 0)
		            ELSE 0 END AS avg_logical_reads,
		       SUM(q.total_elapsed_ms)::bigint AS sum_exec_ms,
		       q.database_name,
		       MAX(q.login_name) AS login_name,
		       MAX(q.application_name) AS program_name,
		       MAX(q.ts) AS last_capture
		FROM sqlserver_query_metrics_v2 q
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = q.instance_id
		 AND class.query_hash = q.query_hash
		WHERE UPPER(q.instance_id) = UPPER($1)
		  AND q.ts >= $2
		  AND q.ts <= $3
		  AND ($5 = '' OR q.database_name = $5)
		  AND COALESCE(q.is_user_workload, 1) = 1
		  AND q.query_text_raw NOT LIKE '%/* SQL_OPTIMA */%'
		  AND (q.login_name IS NULL OR q.login_name <> 'dbmonitor_user')
		  AND UPPER(q.query_text_raw) NOT LIKE 'FETCH NEXT FROM %'
		  AND UPPER(q.query_text_raw) NOT LIKE 'SET %'
		  AND UPPER(q.query_text_raw) NOT LIKE 'DECLARE %'
		  AND UPPER(q.query_text_raw) NOT LIKE '(@%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'CREATE %'
		  AND UPPER(q.query_text_raw) NOT LIKE 'ALTER %'
		  AND UPPER(q.query_text_raw) NOT LIKE 'CHECKPOINT%'
		  AND UPPER(q.query_text_raw) NOT LIKE 'DBCC %'
		  AND COALESCE(q.application_name, '') NOT LIKE 'Microsoft SQL Server Management Studio%'
		  AND COALESCE(q.application_name, '') NOT LIKE 'SQLAgent%'
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		GROUP BY q.query_hash, q.database_name
		ORDER BY SUM(q.total_cpu_ms) DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, start, end, limit, strings.TrimSpace(database))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var qh int64
		var queryText sql.NullString
		var totalExecutions, sumCPU, sumReads, sumExec int64
		var avgCpuMs, avgLogicalReads float64
		var dbName, loginName, programName sql.NullString
		var lastCap time.Time

		if err := rows.Scan(&qh, &queryText, &totalExecutions, &sumCPU, &avgCpuMs, &sumReads, &avgLogicalReads, &sumExec,
			&dbName, &loginName, &programName, &lastCap); err != nil {
			log.Printf("[TSLogger] GetSQLServerTopQueriesWithRange scan: %v", err)
			continue
		}

		totalCpuF := float64(sumCPU)
		totalReadsF := float64(sumReads)
		totalExecF := float64(sumExec)

		results = append(results, map[string]interface{}{
			"capture_timestamp":   lastCap,
			"timestamp":           lastCap,
			"query_hash":          fmt.Sprintf("0x%X", uint64(qh)),
			"query_text":          queryText.String,
			"execution_count":     totalExecutions,
			"Executions":          totalExecutions,
			"avg_cpu_ms":          avgCpuMs,
			"total_cpu_ms":        totalCpuF,
			"total_cpu_time_ms":   totalCpuF,
			"avg_logical_reads":   avgLogicalReads,
			"total_logical_reads": totalReadsF,
			"exec_time_ms":        totalExecF,
			"total_exec_time_ms":  totalExecF,
			"database_name":       dbName.String,
			"login_name":          loginName.String,
			"program_name":        programName.String,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) LogSQLServerLongRunningQueries(ctx context.Context, instanceName string, queries []LongRunningQueryRow) error {
	if len(queries) == 0 {
		return nil
	}

	tl.mu.Lock()
	defer tl.mu.Unlock()

	batch := &pgx.Batch{}
	now := time.Now().UTC()
	queued := 0

	for _, q := range queries {
		key := q.QueryHash
		if key == "" {
			key = fmt.Sprintf("%d-%d", q.SessionID, q.RequestID)
		}

		prevElapsed, exists := tl.prevLongRunningHash[key]
		if exists && int64(q.TotalElapsedTimeMs)-prevElapsed < 5000 {
			continue
		}

		var qh []byte
		hexStr := strings.TrimPrefix(q.QueryHash, "0x")
		if hexStr != "" {
			if b, err := hex.DecodeString(hexStr); err == nil {
				qh = b
			} else {
				// Fallback if not even length or invalid hex
				if h, err := strconv.ParseUint(hexStr, 16, 64); err == nil {
					qh = make([]byte, 8)
					binary.BigEndian.PutUint64(qh, h)
				}
			}
		}

		batch.Queue(`INSERT INTO sqlserver_long_running_queries (
			capture_timestamp, server_instance_name, session_id, request_id,
			database_name, login_name, host_name, program_name,
			query_hash, query_text, wait_type, blocking_session_id, status,
			cpu_time_ms, total_elapsed_time_ms, reads, writes,
			granted_query_memory_mb, row_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			now, instanceName, q.SessionID, q.RequestID,
			tl.ToSafeUTF8(q.DatabaseName), tl.ToSafeUTF8(q.LoginName), tl.ToSafeUTF8(q.HostName), tl.ToSafeUTF8(q.ProgramName),
			qh, tl.ToSafeUTF8(q.QueryText), tl.ToSafeUTF8(q.WaitType), q.BlockingSessionID, tl.ToSafeUTF8(q.Status),
			q.CPUTimeMs, q.TotalElapsedTimeMs, q.Reads, q.Writes,
			q.GrantedQueryMemoryMB, q.RowCount,
		)
		queued++

		tl.prevLongRunningHash[key] = int64(q.TotalElapsedTimeMs)
	}

	if queued == 0 {
		return nil
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < queued; i++ {
		if _, err := br.Exec(); err != nil {
			log.Printf("[TSLogger] Failed to execute batch for long running queries: %v", err)
		}
	}

	return nil
}

func (tl *TimescaleLogger) GetSQLServerLongRunningQueries(ctx context.Context, instanceName string, limit int, from, to string, database string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRange(from, to)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, session_id, request_id, database_name,
		       login_name, host_name, program_name, query_hash, query_text,
		       wait_type, blocking_session_id, status,
		       cpu_time_ms, total_elapsed_time_ms, reads, writes,
		       granted_query_memory_mb, row_count
		FROM sqlserver_long_running_queries
		WHERE UPPER(server_instance_name) = UPPER($1)
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND ($4::text IS NULL OR $4 = '' OR database_name = $4)
		  AND query_text NOT LIKE '%/* SQL_OPTIMA */%'
		  AND (login_name IS NULL OR login_name <> 'dbmonitor_user')
		ORDER BY capture_timestamp DESC, total_elapsed_time_ms DESC
		LIMIT $5
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, start, end, database, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var sessionID, requestID int
		var databaseName, loginName, hostName, programName string
		var qhBytes []byte
		var queryText sql.NullString
		var waitType string
		var blockingSessionID int
		var status string
		var cpuTimeMs, totalElapsedTimeMs int
		var reads, writes int
		var grantedQueryMemoryMB int
		var rowCount int

		if err := rows.Scan(&ts, &sessionID, &requestID, &databaseName,
			&loginName, &hostName, &programName, &qhBytes, &queryText,
			&waitType, &blockingSessionID, &status,
			&cpuTimeMs, &totalElapsedTimeMs, &reads, &writes,
			&grantedQueryMemoryMB, &rowCount); err != nil {
			continue
		}

		var qhStr string
		if len(qhBytes) > 0 {
			qhStr = "0x" + strings.ToUpper(hex.EncodeToString(qhBytes))
		} else {
			qhStr = "0x0"
		}

		results = append(results, map[string]interface{}{
			"timestamp":               ts,
			"session_id":              sessionID,
			"request_id":              requestID,
			"database_name":           databaseName,
			"login_name":              loginName,
			"host_name":               hostName,
			"program_name":            programName,
			"query_hash":              qhStr,
			"query_text":              queryText.String,
			"wait_type":               waitType,
			"blocking_session_id":     blockingSessionID,
			"status":                  status,
			"cpu_time_ms":             cpuTimeMs,
			"total_elapsed_time_ms":   totalElapsedTimeMs,
			"reads":                   reads,
			"writes":                  writes,
			"granted_query_memory_mb": grantedQueryMemoryMB,
			"row_count":               rowCount,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) LogQueryStoreStatsDirect(ctx context.Context, rows []QueryStoreStatsRow) error {
	if len(rows) == 0 {
		return nil
	}

	// 1. Insert into Staging (Raw Cumulative Data)
	batch := &pgx.Batch{}
	serverName := rows[0].ServerName

	for _, r := range rows {
		var qh, ph int64
		// If models still have hex strings, parse them. Otherwise parse decimal string representation.
		if h, err := strconv.ParseUint(strings.TrimPrefix(r.QueryHash, "0x"), 16, 64); err == nil {
			qh = int64(h)
		} else if h, err := strconv.ParseInt(r.QueryHash, 10, 64); err == nil {
			qh = h
		}
		
		// r.PlanID is int64 now, so convert it directly.
		ph = r.PlanID

		batch.Queue(`INSERT INTO monitor.sqlserver_query_store_staging (
			server_instance_name, database_name, query_hash, query_text, 
			plan_id, runtime_stats_interval_id, executions, 
			avg_duration_ms, avg_cpu_ms, avg_logical_reads, total_cpu_ms, last_execution_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			r.ServerName, r.DatabaseName, qh, r.QueryText,
			ph, r.IntervalID, r.Executions,
			r.AvgDurationMs, r.AvgCpuMs, r.AvgLogicalReads, r.TotalCpuMs, r.LastExecutionTime,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		return fmt.Errorf("query store staging batch: %w", err)
	}

	// 2. Process Snapshot (Deduplicate and store unique states)
	if err := tl.ProcessQueryStoreSnapshot(ctx, serverName); err != nil {
		log.Printf("[TSLogger] ProcessQueryStoreSnapshot failed for %s: %v", serverName, err)
	}

	// 3. Process Delta (Calculate subtraction from previous snapshot)
	if err := tl.ProcessQueryStoreDelta(ctx, serverName); err != nil {
		log.Printf("[TSLogger] ProcessQueryStoreDelta failed for %s: %v", serverName, err)
	}

	return nil
}

func (tl *TimescaleLogger) ProcessQueryStoreSnapshot(ctx context.Context, instanceName string) error {
	query := `
		INSERT INTO monitor.sqlserver_query_store_snapshot (
			capture_time, server_instance_name, database_name, query_hash, query_text,
			plan_id, runtime_stats_interval_id, total_executions, total_cpu_ms,
			total_duration_ms, total_logical_reads, row_fingerprint
		)
		SELECT
			NOW(),
			s.server_instance_name,
			s.database_name,
			s.query_hash,
			s.query_text,
			s.plan_id,
			s.runtime_stats_interval_id,
			s.executions,
			s.total_cpu_ms,
			(s.avg_duration_ms * s.executions),
			(s.avg_logical_reads * s.executions),
			md5(s.executions::text || '-' || s.total_cpu_ms::text || '-' || s.runtime_stats_interval_id::text)
		FROM monitor.sqlserver_query_store_staging s
		WHERE UPPER(s.server_instance_name) = UPPER($1)
		AND NOT EXISTS (
			SELECT 1
			FROM (
				SELECT last.row_fingerprint
				FROM monitor.sqlserver_query_store_snapshot last
				WHERE last.server_instance_name = s.server_instance_name
				  AND last.query_hash = s.query_hash
				  AND last.plan_id = s.plan_id
				  AND last.runtime_stats_interval_id = s.runtime_stats_interval_id
				ORDER BY last.capture_time DESC
				LIMIT 1
			) prev
			WHERE prev.row_fingerprint = md5(s.executions::text || '-' || s.total_cpu_ms::text || '-' || s.runtime_stats_interval_id::text)
		)
		ON CONFLICT DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query, instanceName)
	if err == nil {
		_, _ = tl.pool.Exec(ctx, `DELETE FROM monitor.sqlserver_query_store_staging WHERE UPPER(server_instance_name) = UPPER($1)`, instanceName)
	}
	return err
}

func (tl *TimescaleLogger) ProcessQueryStoreDelta(ctx context.Context, instanceName string) error {
	query := `
		INSERT INTO monitor.sqlserver_query_store_interval (
			bucket_start, bucket_end, server_instance_name, database_name, query_hash, query_text,
			plan_id, runtime_stats_interval_id, delta_executions, delta_cpu_ms,
			delta_duration_ms, delta_logical_reads, avg_cpu_ms, avg_duration_ms, avg_reads, is_reset
		)
		SELECT
			t.prev_time,
			t.capture_time,
			t.server_instance_name,
			t.database_name,
			t.query_hash,
			t.query_text,
			t.plan_id,
			t.runtime_stats_interval_id,
			t.exec_delta,
			t.cpu_delta,
			t.dur_delta,
			t.reads_delta,
			(t.cpu_delta / NULLIF(t.exec_delta, 0)),
			(t.dur_delta / NULLIF(t.exec_delta, 0)),
			(t.reads_delta / NULLIF(t.exec_delta, 0)),
			t.reset
		FROM (
			SELECT
				curr.*,
				prev.capture_time AS prev_time,
				curr.total_executions - COALESCE(prev.total_executions, 0) AS exec_delta,
				curr.total_cpu_ms - COALESCE(prev.total_cpu_ms, 0) AS cpu_delta,
				curr.total_duration_ms - COALESCE(prev.total_duration_ms, 0) AS dur_delta,
				curr.total_logical_reads - COALESCE(prev.total_logical_reads, 0) AS reads_delta,
				(curr.total_executions < COALESCE(prev.total_executions, 0) OR curr.runtime_stats_interval_id != prev.runtime_stats_interval_id) AS reset
			FROM monitor.sqlserver_query_store_snapshot curr
			-- Join with metrics to get query_text_raw for filtering tool-generated queries
			LEFT JOIN (
				SELECT DISTINCT query_hash, query_text_raw, total_cpu_ms, total_elapsed_ms
				FROM sqlserver_query_metrics_v2
				WHERE UPPER(instance_id) = UPPER($1)
			) sqm ON curr.query_hash = sqm.query_hash
			JOIN LATERAL (
				SELECT total_executions, total_cpu_ms, total_duration_ms, total_logical_reads, capture_time, runtime_stats_interval_id
				FROM monitor.sqlserver_query_store_snapshot p
				WHERE p.server_instance_name = curr.server_instance_name
				  AND p.query_hash = curr.query_hash
				  AND p.plan_id = curr.plan_id
				  AND p.capture_time < curr.capture_time
				ORDER BY capture_time DESC
				LIMIT 1
			) prev ON true
			WHERE UPPER(curr.server_instance_name) = UPPER($1)
			  AND (sqm.query_text_raw IS NULL OR (
				sqm.query_text_raw NOT LIKE '%/* SQL_OPTIMA%' AND
				sqm.query_text_raw NOT LIKE '%sys.all_objects%' AND
				sqm.query_text_raw NOT LIKE '(@_msparam_0%' AND
				(sqm.total_cpu_ms > 20 OR sqm.total_elapsed_ms > 20)
			  ))
		) t
		WHERE t.exec_delta > 0
		ON CONFLICT (bucket_end, server_instance_name, query_hash, plan_id, runtime_stats_interval_id) DO NOTHING
	`
	_, err := tl.pool.Exec(ctx, query, instanceName)
	return err
}

func (tl *TimescaleLogger) LogQueryStoreStats(ctx context.Context, instanceName string, queries []QueryStoreStatsRow) error {
	return tl.LogQueryStoreStatsDirect(ctx, queries)
}

func (tl *TimescaleLogger) GetQueryStoreStats(ctx context.Context, instanceName string, timeRange string, limit int) ([]QueryStoreStatsRow, error) {
	if limit <= 0 {
		limit = 100
	}

	var interval string
	switch timeRange {
	case "1h":
		interval = "1 hour"
	case "24h":
		interval = "24 hours"
	case "7d":
		interval = "7 days"
	default:
		interval = "1 hour"
	}

	query := fmt.Sprintf(`
		SELECT q.ts as bucket_end, q.instance_id as server_instance_name, q.database_name,
		       q.query_hash, q.statement_text as query_text, q.total_executions as delta_executions,
		       (q.total_elapsed_ms::float8 / NULLIF(q.total_executions, 0)) as avg_duration_ms,
		       (q.total_cpu_ms::float8 / NULLIF(q.total_executions, 0)) as avg_cpu_ms,
		       q.total_logical_reads as delta_logical_reads, q.total_cpu_ms as delta_cpu_ms
		FROM sqlserver_query_metrics_v2 q
		LEFT JOIN sqlserver_query_classification_dim class
		  ON class.instance_id = q.instance_id
		 AND class.query_hash = q.query_hash
		WHERE UPPER(q.instance_id) = UPPER($1)
		  AND q.ts >= NOW() - INTERVAL '%s'
		  AND COALESCE(q.is_user_workload, 1) = 1
		  AND q.query_text_raw NOT LIKE '%%/* SQL_OPTIMA%%'
		  AND q.query_text_raw NOT LIKE '%%sys.all_objects%%'
		  AND q.query_text_raw NOT LIKE '(@_msparam_0%%'
		  AND (q.total_cpu_ms > 20 OR q.total_elapsed_ms > 20)
		  AND (q.login_name IS NULL OR q.login_name <> 'dbmonitor_user')
		  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
		ORDER BY q.ts DESC, q.total_cpu_ms DESC
		LIMIT $2
	`, interval)
	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []QueryStoreStatsRow
	for rows.Next() {
		var r QueryStoreStatsRow
		var qh int64
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.DatabaseName,
			&qh, &r.QueryText, &r.Executions,
			&r.AvgDurationMs, &r.AvgCpuMs, &r.AvgLogicalReads, &r.TotalCpuMs); err != nil {
			continue
		}
		r.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetQueryStoreBottlenecks(ctx context.Context, instanceName string, timeRange string, limit int, database string, from, to string) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}

	var start, end time.Time
	var err error
	useRange := false
	if from != "" && to != "" {
		start, end, err = parseTimeRangeRFC3339(from, to)
		if err == nil {
			useRange = true
		}
	}

	var query string
	if useRange {
		query = `
			WITH recent AS (
				SELECT q.query_hash, MAX(q.query_text) as query_text, q.database_name,
				       SUM(q.delta_executions) as total_exec,
				       AVG(q.delta_cpu_ms::float8 / NULLIF(q.delta_executions, 0)) as avg_cpu,
				       SUM(q.delta_cpu_ms)::float8 as total_cpu,
				       AVG(q.delta_duration_ms::float8 / NULLIF(q.delta_executions, 0)) as avg_dur,
				       AVG(q.delta_logical_reads::float8 / NULLIF(q.delta_executions, 0)) as avg_reads,
				       CASE 
				         WHEN AVG(q.delta_cpu_ms::float8 / NULLIF(q.delta_executions, 0)) > 0.7 * AVG(q.delta_duration_ms::float8 / NULLIF(q.delta_executions, 0)) THEN 'CPU'
				         WHEN AVG(q.delta_logical_reads::float8 / NULLIF(q.delta_executions, 0)) > 5000 THEN 'I/O'
				         ELSE 'Wait'
				       END as bottleneck_type
				FROM monitor.sqlserver_query_store_interval q
				LEFT JOIN sqlserver_query_classification_dim class
				  ON class.instance_id = q.server_instance_name
				 AND class.query_hash = q.query_hash
				WHERE UPPER(q.server_instance_name) = UPPER($1)
				  AND q.bucket_end >= $2 AND q.bucket_end <= $3
				  AND ($4::text IS NULL OR $4 = '' OR q.database_name = $4)
				  AND q.query_text NOT LIKE '%/* SQL_OPTIMA %'
				  AND (q.delta_cpu_ms > 20 OR q.delta_duration_ms > 20)
				  AND UPPER(q.query_text) NOT LIKE 'FETCH NEXT FROM %'
				  AND UPPER(q.query_text) NOT LIKE 'SET %'
				  AND UPPER(q.query_text) NOT LIKE 'DECLARE %'
				  AND UPPER(q.query_text) NOT LIKE '(@%'
				  AND UPPER(q.query_text) NOT LIKE 'CREATE %'
				  AND UPPER(q.query_text) NOT LIKE 'ALTER %'
				  AND UPPER(q.query_text) NOT LIKE 'CHECKPOINT%'
				  AND UPPER(q.query_text) NOT LIKE 'DBCC %'
				  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
				GROUP BY q.query_hash, q.database_name
			)
			SELECT query_hash, query_text, database_name, total_exec::bigint, 
			       COALESCE(avg_cpu, 0), COALESCE(total_cpu, 0), COALESCE(avg_dur, 0), COALESCE(avg_reads, 0),
			       bottleneck_type
			FROM recent
			WHERE total_exec > 0
			ORDER BY total_cpu DESC
			LIMIT $5
		`
	} else {
		var interval string
		switch timeRange {
		case "15m":
			interval = "15 minutes"
		case "1h":
			interval = "1 hour"
		case "6h":
			interval = "6 hours"
		case "24h":
			interval = "24 hours"
		case "7d":
			interval = "7 days"
		default:
			interval = "1 hour"
		}

		// Querying from the true delta interval table
		query = fmt.Sprintf(`
			WITH recent AS (
				SELECT q.query_hash, MAX(q.query_text) as query_text, q.database_name,
				       SUM(q.delta_executions) as total_exec,
				       AVG(q.delta_cpu_ms::float8 / NULLIF(q.delta_executions, 0)) as avg_cpu,
				       SUM(q.delta_cpu_ms)::float8 as total_cpu,
				       AVG(q.delta_duration_ms::float8 / NULLIF(q.delta_executions, 0)) as avg_dur,
				       AVG(q.delta_logical_reads::float8 / NULLIF(q.delta_executions, 0)) as avg_reads,
				       CASE 
				         WHEN AVG(q.delta_cpu_ms::float8 / NULLIF(q.delta_executions, 0)) > 0.7 * AVG(q.delta_duration_ms::float8 / NULLIF(q.delta_executions, 0)) THEN 'CPU'
				         WHEN AVG(q.delta_logical_reads::float8 / NULLIF(q.delta_executions, 0)) > 5000 THEN 'I/O'
				         ELSE 'Wait'
				       END as bottleneck_type
				FROM monitor.sqlserver_query_store_interval q
				LEFT JOIN sqlserver_query_classification_dim class
				  ON class.instance_id = q.server_instance_name
				 AND class.query_hash = q.query_hash
				WHERE UPPER(q.server_instance_name) = UPPER($1)
				  AND q.bucket_end >= NOW() - INTERVAL '%s'
				  AND ($2::text IS NULL OR $2 = '' OR q.database_name = $2)
				  AND q.query_text NOT LIKE '%%%%/* SQL_OPTIMA %%%%'
				  AND (q.delta_cpu_ms > 20 OR q.delta_duration_ms > 20)
				  AND UPPER(q.query_text) NOT LIKE 'FETCH NEXT FROM %%%%'
				  AND UPPER(q.query_text) NOT LIKE 'SET %%%%'
				  AND UPPER(q.query_text) NOT LIKE 'DECLARE %%%%'
				  AND UPPER(q.query_text) NOT LIKE '(@%%%%'
				  AND UPPER(q.query_text) NOT LIKE 'CREATE %%%%'
				  AND UPPER(q.query_text) NOT LIKE 'ALTER %%%%'
				  AND UPPER(q.query_text) NOT LIKE 'CHECKPOINT%%%%'
				  AND UPPER(q.query_text) NOT LIKE 'DBCC %%%%'
				  AND COALESCE(class.classification, 'UNKNOWN') <> 'SYSTEM'
				GROUP BY q.query_hash, q.database_name
			)
			SELECT query_hash, query_text, database_name, total_exec::bigint, 
			       COALESCE(avg_cpu, 0), COALESCE(total_cpu, 0), COALESCE(avg_dur, 0), COALESCE(avg_reads, 0),
			       bottleneck_type
			FROM recent
			WHERE total_exec > 0
			ORDER BY total_cpu DESC
			LIMIT $3
		`, interval)
	}

	var rows pgx.Rows
	if useRange {
		rows, err = tl.pool.Query(ctx, query, instanceName, start, end, database, limit)
	} else {
		rows, err = tl.pool.Query(ctx, query, instanceName, database, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var qh int64
		var queryText sql.NullString
		var databaseName sql.NullString
		var totalExec int64
		var avgCpu, totalCpu, avgDur, avgReads float64
		var bottleneckType string

		if err := rows.Scan(&qh, &queryText, &databaseName, &totalExec, &avgCpu, &totalCpu, &avgDur, &avgReads, &bottleneckType); err != nil {
			log.Printf("[TSLogger] GetQueryStoreBottlenecks scan error: %v", err)
			continue
		}

		results = append(results, map[string]interface{}{
			"query_hash":        fmt.Sprintf("0x%X", uint64(qh)),
			"query_text":        queryText.String,
			"database_name":     databaseName.String,
			"execution_count":   totalExec,
			"avg_cpu_ms":        avgCpu,
			"total_cpu_ms":      totalCpu,
			"avg_duration_ms":   avgDur,
			"avg_logical_reads": avgReads,
			"bottleneck_type":   bottleneckType,
		})
	}
	return results, rows.Err()
}
