package hot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) LogSQLServerTopQueries(ctx context.Context, instanceName string, queries []map[string]interface{}) error {
	if len(queries) == 0 {
		return nil
	}

	tl.mu.Lock()
	defer tl.mu.Unlock()

	now := time.Now().UTC()

	// Rows from FetchTopCPUQueries are already baseline-or-delta shaped; persist them using the
	// actual Timescale schema (cpu_time_ms, exec_time_ms, logical_reads, execution_count, …).
	const insertQuery = `INSERT INTO sqlserver_top_queries (
			capture_timestamp, server_instance_name, login_name, program_name, database_name, query_text,
			cpu_time_ms, exec_time_ms, logical_reads, execution_count, query_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	for _, q := range queries {
		queryHash := strings.TrimSpace(getStr(q, "Query_Hash"))
		if queryHash == "" {
			queryHash = strings.TrimSpace(fmt.Sprintf("%v", q["Query_Hash"]))
		}
		if queryHash == "" {
			continue
		}

		execCount := getInt64(q, "Executions")
		if execCount < 0 {
			execCount = 0
		}
		cpuMs := getFloat64(q, "Total_CPU_ms")
		logicalReads := getInt64(q, "Total_Logical_Reads")
		elapsedMs := getFloat64(q, "Total_Elapsed_ms")

		login := getStr(q, "Login_Name")
		if login == "" {
			login = getStr(q, "login_name")
		}
		prog := getStr(q, "Client_App")
		if prog == "" {
			prog = getStr(q, "program_name")
		}
		dbName := getStr(q, "Database_Name")
		if dbName == "" {
			dbName = getStr(q, "database_name")
		}
		qtext := getStr(q, "Query_Text")
		if qtext == "" {
			qtext = getStr(q, "query_text")
		}

		cpuMillis := int64(cpuMs + 0.5)
		execMillis := int64(elapsedMs + 0.5)

		if _, err := tl.pool.Exec(ctx, insertQuery,
			now, instanceName, login, prog, dbName, qtext,
			cpuMillis, execMillis, logicalReads, execCount, queryHash,
		); err != nil {
			log.Printf("[TSLogger] Failed to log top query: %v", err)
		}
	}

	return nil
}

func (tl *TimescaleLogger) GetSQLServerTopQueries(ctx context.Context, instanceName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT capture_timestamp, query_hash, query_text, execution_count,
		       cpu_time_ms, exec_time_ms, logical_reads,
		       database_name, login_name, program_name
		FROM sqlserver_top_queries
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC, cpu_time_ms DESC
		LIMIT $2
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var queryHash string
		var queryText sql.NullString
		var dbName, loginName, programName sql.NullString
		var execCount, cpuTimeMs, execTimeMs, logicalReads int64

		if err := rows.Scan(&ts, &queryHash, &queryText, &execCount, &cpuTimeMs, &execTimeMs, &logicalReads,
			&dbName, &loginName, &programName); err != nil {
			continue
		}

		var avgCpu, avgReads float64
		if execCount > 0 {
			avgCpu = float64(cpuTimeMs) / float64(execCount)
			avgReads = float64(logicalReads) / float64(execCount)
		}

		results = append(results, map[string]interface{}{
			"capture_timestamp":   ts,
			"timestamp":           ts,
			"query_hash":          queryHash,
			"query_text":          queryText.String,
			"execution_count":     execCount,
			"avg_cpu_ms":          avgCpu,
			"total_cpu_ms":        float64(cpuTimeMs),
			"total_cpu_time_ms":   float64(cpuTimeMs),
			"exec_time_ms":        float64(execTimeMs),
			"total_exec_time_ms":  float64(execTimeMs),
			"avg_logical_reads":   avgReads,
			"total_logical_reads": float64(logicalReads),
			"database_name":       dbName.String,
			"login_name":          loginName.String,
			"program_name":        programName.String,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerTopQueriesWithRange(ctx context.Context, instanceName string, limit int, from, to string) ([]map[string]interface{}, error) {
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

	// Roll up entire window per query_hash (no time_bucket — works on plain PostgreSQL).
	query := `
		SELECT query_hash,
		       (array_agg(query_text ORDER BY capture_timestamp DESC)
		          FILTER (WHERE query_text IS NOT NULL AND trim(query_text) <> ''))[1] AS query_text,
		       SUM(execution_count)::bigint AS total_executions,
		       SUM(cpu_time_ms)::bigint AS sum_cpu_ms,
		       CASE WHEN SUM(execution_count) > 0
		            THEN SUM(cpu_time_ms)::float8 / NULLIF(SUM(execution_count)::float8, 0)
		            ELSE 0 END AS avg_cpu_ms,
		       SUM(logical_reads)::bigint AS sum_logical_reads,
		       CASE WHEN SUM(execution_count) > 0
		            THEN SUM(logical_reads)::float8 / NULLIF(SUM(execution_count)::float8, 0)
		            ELSE 0 END AS avg_logical_reads,
		       SUM(COALESCE(exec_time_ms, 0))::bigint AS sum_exec_ms,
		       MAX(database_name) AS database_name,
		       MAX(login_name) AS login_name,
		       MAX(program_name) AS program_name,
		       MAX(capture_timestamp) AS last_capture
		FROM sqlserver_top_queries
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		GROUP BY query_hash
		ORDER BY SUM(cpu_time_ms) DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var queryHash string
		var queryText sql.NullString
		var totalExecutions, sumCPU, sumReads, sumExec int64
		var avgCpuMs, avgLogicalReads float64
		var dbName, loginName, programName sql.NullString
		var lastCap time.Time

		if err := rows.Scan(&queryHash, &queryText, &totalExecutions, &sumCPU, &avgCpuMs, &sumReads, &avgLogicalReads, &sumExec,
			&dbName, &loginName, &programName, &lastCap); err != nil {
			continue
		}

		totalCpuF := float64(sumCPU)
		totalReadsF := float64(sumReads)
		totalExecF := float64(sumExec)

		results = append(results, map[string]interface{}{
			"capture_timestamp":   lastCap,
			"timestamp":           lastCap,
			"query_hash":          queryHash,
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
		// Delta-style de-dupe similar to Top CPU queries:
		// Prefer stable query_hash so repeated samples of same query/SP group together.
		// Fall back to session/request if hash is missing.
		key := q.QueryHash
		if key == "" {
			key = fmt.Sprintf("%d-%d", q.SessionID, q.RequestID)
		}

		prevElapsed, exists := tl.prevLongRunningHash[key]
		// Only log when elapsed meaningfully advances to avoid repeated inserts every tick.
		// (SQL elapsed time is in ms)
		if exists && int64(q.TotalElapsedTimeMs)-prevElapsed < 5000 {
			continue
		}

		batch.Queue(`INSERT INTO sqlserver_long_running_queries (
			capture_timestamp, server_instance_name, session_id, request_id,
			database_name, login_name, host_name, program_name,
			query_hash, query_text, wait_type, blocking_session_id, status,
			cpu_time_ms, total_elapsed_time_ms, reads, writes,
			granted_query_memory_mb, row_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			now, instanceName, q.SessionID, q.RequestID,
			q.DatabaseName, q.LoginName, q.HostName, q.ProgramName,
			q.QueryHash, q.QueryText, q.WaitType, q.BlockingSessionID, q.Status,
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
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		  AND ($4::text IS NULL OR $4 = '' OR database_name = $4)
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
		var queryHash sql.NullString
		var queryText sql.NullString
		var waitType string
		var blockingSessionID int
		var status string
		var cpuTimeMs, totalElapsedTimeMs int
		var reads, writes int
		var grantedQueryMemoryMB int
		var rowCount int

		if err := rows.Scan(&ts, &sessionID, &requestID, &databaseName,
			&loginName, &hostName, &programName, &queryHash, &queryText,
			&waitType, &blockingSessionID, &status,
			&cpuTimeMs, &totalElapsedTimeMs, &reads, &writes,
			&grantedQueryMemoryMB, &rowCount); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":               ts,
			"session_id":              sessionID,
			"request_id":              requestID,
			"database_name":           databaseName,
			"login_name":              loginName,
			"host_name":               hostName,
			"program_name":            programName,
			"query_hash":              queryHash.String,
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

	batch := &pgx.Batch{}
	now := time.Now().UTC()

	for _, r := range rows {
		ts := r.CaptureTimestamp
		if ts.IsZero() {
			ts = now
		}
		tsu := ts.UTC()
		batch.Queue(`INSERT INTO query_store_stats (
			capture_timestamp, server_name, database_name,
			query_hash, query_text, executions,
			avg_duration_ms, avg_cpu_ms, avg_logical_reads, total_cpu_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			tsu, r.ServerName, r.DatabaseName,
			r.QueryHash, r.QueryText, r.Executions,
			r.AvgDurationMs, r.AvgCpuMs, r.AvgLogicalReads, r.TotalCpuMs,
		)
		// Enterprise hypertable (same snapshot; used by external scripts / ops expecting this name).
		batch.Queue(`INSERT INTO sqlserver_query_store_stats (
			capture_timestamp, server_instance_name, database_name,
			query_hash, query_text, executions,
			avg_duration_ms, avg_cpu_ms, avg_logical_reads, total_cpu_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			tsu, r.ServerName, r.DatabaseName,
			r.QueryHash, r.QueryText, r.Executions,
			r.AvgDurationMs, r.AvgCpuMs, r.AvgLogicalReads, r.TotalCpuMs,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(rows)*2; i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("query store batch op %d: %w", i, err)
		}
	}

	return nil
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
		SELECT capture_timestamp, server_name, database_name,
		       query_hash, query_text, executions,
		       avg_duration_ms, avg_cpu_ms, avg_logical_reads, total_cpu_ms
		FROM query_store_stats
		WHERE server_name = $1
		  AND capture_timestamp >= NOW() - INTERVAL '%s'
		ORDER BY capture_timestamp DESC, total_cpu_ms DESC
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
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerName, &r.DatabaseName,
			&r.QueryHash, &r.QueryText, &r.Executions,
			&r.AvgDurationMs, &r.AvgCpuMs, &r.AvgLogicalReads, &r.TotalCpuMs); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetQueryStoreBottlenecks(ctx context.Context, instanceName string, timeRange string, limit int, database string) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 50
	}

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

	query := fmt.Sprintf(`
		WITH recent AS (
			SELECT query_hash, query_text, database_name,
			       SUM(executions) as total_exec,
			       AVG(avg_cpu_ms) as avg_cpu,
			       SUM(total_cpu_ms) as total_cpu,
			       AVG(avg_duration_ms) as avg_dur,
			       AVG(avg_logical_reads) as avg_reads
			FROM query_store_stats
			WHERE server_name = $1
			  AND capture_timestamp >= NOW() - INTERVAL '%s'
			  AND ($2::text IS NULL OR $2 = '' OR database_name = $2)
			GROUP BY query_hash, query_text, database_name
		)
		SELECT query_hash, query_text, database_name, total_exec, avg_cpu, total_cpu, avg_dur, avg_reads,
		       CASE WHEN avg_dur > 1000 THEN 'high_duration'
		            WHEN avg_cpu > 500 THEN 'high_cpu'
		            WHEN avg_reads > 100000 THEN 'high_io'
		            ELSE 'normal' END as bottleneck_type
		FROM recent
		WHERE total_exec > 0
		ORDER BY total_cpu DESC
		LIMIT $3
	`, interval)

	rows, err := tl.pool.Query(ctx, query, instanceName, database, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var queryHash string
		var queryText sql.NullString
		var databaseName sql.NullString
		var totalExec int64
		var avgCpu, totalCpu, avgDur, avgReads float64
		var bottleneckType string

		if err := rows.Scan(&queryHash, &queryText, &databaseName, &totalExec, &avgCpu, &totalCpu, &avgDur, &avgReads, &bottleneckType); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"query_hash":        queryHash,
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

func getInt64(m map[string]interface{}, key string) int64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int64:
			return val
		case int:
			return int64(val)
		case float64:
			return int64(val)
		}
	}
	return 0
}

// LogQueryStatsStaging inserts DMV snapshot rows for the change-only snapshot + delta pipeline.
func (tl *TimescaleLogger) LogQueryStatsStaging(ctx context.Context, instanceName string, queries []map[string]interface{}) error {
	if len(queries) == 0 {
		return nil
	}

	tl.mu.Lock()
	defer tl.mu.Unlock()

	now := time.Now().UTC()

	batch := &pgx.Batch{}
	for _, q := range queries {
		batch.Queue(`INSERT INTO sqlserver_query_stats_staging 
			(capture_time, server_instance_name, database_name, login_name, client_app, query_hash, query_text, 
			 total_executions, total_cpu_ms, total_elapsed_ms, total_logical_reads, total_physical_reads, total_rows)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			now, instanceName,
			getStr(q, "database_name"),
			getStr(q, "login_name"),
			getStr(q, "client_app"),
			getStr(q, "query_hash"),
			getStr(q, "query_text"),
			getInt64(q, "total_executions"),
			getInt64(q, "total_cpu_ms"),
			getInt64(q, "total_elapsed_ms"),
			getInt64(q, "total_logical_reads"),
			getInt64(q, "total_physical_reads"),
			getInt64(q, "total_rows"),
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(queries); i++ {
		if _, err := br.Exec(); err != nil {
			log.Printf("[TSLogger] Failed to insert query stats staging: %v", err)
		}
	}

	return nil
}

func (tl *TimescaleLogger) ProcessQueryStatsSnapshot(ctx context.Context, instanceName string) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	query := `
		INSERT INTO sqlserver_query_stats_snapshot
			(capture_time, server_instance_name, database_name, login_name, client_app, query_hash, query_text,
			 total_executions, total_cpu_ms, total_elapsed_ms, total_logical_reads, total_physical_reads, total_rows, row_fingerprint)
		SELECT
			s.capture_time,
			s.server_instance_name,
			s.database_name,
			s.login_name,
			s.client_app,
			s.query_hash,
			s.query_text,
			s.total_executions,
			s.total_cpu_ms,
			s.total_elapsed_ms,
			s.total_logical_reads,
			s.total_physical_reads,
			s.total_rows,
			md5(s.total_executions::text || '-' || s.total_cpu_ms::text || '-' || s.total_elapsed_ms::text || '-' || 
			    s.total_logical_reads::text || '-' || s.total_physical_reads::text || '-' || s.total_rows::text)
		FROM sqlserver_query_stats_staging s
		WHERE s.server_instance_name = $1
		AND NOT EXISTS (
			SELECT 1
			FROM (
				SELECT last.row_fingerprint
				FROM sqlserver_query_stats_snapshot last
				WHERE last.server_instance_name = s.server_instance_name
				  AND last.query_hash = s.query_hash
				  AND last.database_name IS NOT DISTINCT FROM s.database_name
				  AND last.login_name IS NOT DISTINCT FROM s.login_name
				  AND last.client_app IS NOT DISTINCT FROM s.client_app
				ORDER BY last.capture_time DESC
				LIMIT 1
			) prev
			WHERE prev.row_fingerprint = md5(s.total_executions::text || '-' || s.total_cpu_ms::text || '-' || s.total_elapsed_ms::text || '-' || 
			      s.total_logical_reads::text || '-' || s.total_physical_reads::text || '-' || s.total_rows::text)
		)
		ON CONFLICT DO NOTHING
	`

	_, err := tl.pool.Exec(ctx, query, instanceName)
	if err != nil {
		log.Printf("[TSLogger] Failed to process query stats snapshot: %v", err)
		return err
	}

	_, err = tl.pool.Exec(ctx, `DELETE FROM sqlserver_query_stats_staging WHERE server_instance_name = $1`, instanceName)
	if err != nil {
		log.Printf("[TSLogger] Failed to clear staging table: %v", err)
	}

	return err
}

func (tl *TimescaleLogger) ProcessQueryStatsDelta(ctx context.Context, instanceName string) error {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	query := `
		INSERT INTO sqlserver_query_stats_interval
			(bucket_start, bucket_end, server_instance_name, database_name, login_name, client_app, query_hash, query_text,
			 executions, cpu_ms, duration_ms, logical_reads, physical_reads, rows,
			 avg_cpu_ms, avg_duration_ms, avg_reads, is_reset)
		SELECT
			prev.capture_time,
			curr.capture_time,
			curr.server_instance_name,
			curr.database_name,
			curr.login_name,
			curr.client_app,
			curr.query_hash,
			curr.query_text,
			CASE WHEN reset THEN 0 ELSE exec_delta END,
			CASE WHEN reset THEN 0 ELSE cpu_delta END,
			CASE WHEN reset THEN 0 ELSE dur_delta END,
			CASE WHEN reset THEN 0 ELSE reads_delta END,
			CASE WHEN reset THEN 0 ELSE phys_delta END,
			CASE WHEN reset THEN 0 ELSE rows_delta END,
			cpu_delta / NULLIF(exec_delta, 0)::numeric,
			dur_delta / NULLIF(exec_delta, 0)::numeric,
			reads_delta / NULLIF(exec_delta, 0)::numeric,
			reset
		FROM (
			SELECT
				curr.*,
				prev.capture_time AS prev_time,
				curr.total_executions - COALESCE(prev.total_executions, 0) AS exec_delta,
				curr.total_cpu_ms - COALESCE(prev.total_cpu_ms, 0) AS cpu_delta,
				curr.total_elapsed_ms - COALESCE(prev.total_elapsed_ms, 0) AS dur_delta,
				curr.total_logical_reads - COALESCE(prev.total_logical_reads, 0) AS reads_delta,
				curr.total_physical_reads - COALESCE(prev.total_physical_reads, 0) AS phys_delta,
				curr.total_rows - COALESCE(prev.total_rows, 0) AS rows_delta,
				(curr.total_executions < COALESCE(prev.total_executions, 0)
				 OR curr.total_cpu_ms < COALESCE(prev.total_cpu_ms, 0)) AS reset
			FROM sqlserver_query_stats_snapshot curr
			JOIN LATERAL (
				SELECT total_executions, total_cpu_ms, total_elapsed_ms, total_logical_reads, total_physical_reads, total_rows, capture_time
				FROM sqlserver_query_stats_snapshot p
				WHERE p.server_instance_name = curr.server_instance_name
				  AND p.query_hash = curr.query_hash
				  AND p.database_name IS NOT DISTINCT FROM curr.database_name
				  AND p.login_name IS NOT DISTINCT FROM curr.login_name
				  AND p.client_app IS NOT DISTINCT FROM curr.client_app
				  AND p.capture_time < curr.capture_time
				ORDER BY capture_time DESC
				LIMIT 1
			) prev ON true
			WHERE curr.server_instance_name = $1
		) t
		WHERE exec_delta > 0 OR cpu_delta > 0 OR dur_delta > 0
		ON CONFLICT (bucket_end, query_hash, database_name, login_name, client_app, server_instance_name) DO UPDATE SET
			executions = sqlserver_query_stats_interval.executions + EXCLUDED.executions,
			cpu_ms = sqlserver_query_stats_interval.cpu_ms + EXCLUDED.cpu_ms,
			duration_ms = sqlserver_query_stats_interval.duration_ms + EXCLUDED.duration_ms,
			logical_reads = sqlserver_query_stats_interval.logical_reads + EXCLUDED.logical_reads,
			physical_reads = sqlserver_query_stats_interval.physical_reads + EXCLUDED.physical_reads,
			rows = sqlserver_query_stats_interval.rows + EXCLUDED.rows
	`

	_, err := tl.pool.Exec(ctx, query, instanceName)
	if err != nil {
		log.Printf("[TSLogger] Failed to process query stats delta: %v", err)
		return err
	}

	return nil
}

// QueryStatsDashboardParams drives leaderboard queries over sqlserver_query_stats_interval.
type QueryStatsDashboardParams struct {
	InstanceName string
	Metric       string // cpu, duration, reads, executions
	TimeRange    string // 15m, 1h, 24h (used when From/To are empty)
	Dimension    string // query, database, login, app
	Limit        int
	From         string // RFC3339 inclusive lower bound for bucket_end (optional)
	To           string // RFC3339 inclusive upper bound for bucket_end (optional)
}

func (tl *TimescaleLogger) GetQueryStatsDashboard(ctx context.Context, params QueryStatsDashboardParams) ([]map[string]interface{}, error) {
	if params.Limit <= 0 {
		params.Limit = 20
	}

	timeRange := map[string]string{
		"15m": "15 minutes",
		"1h":  "1 hour",
		"24h": "24 hours",
	}[params.TimeRange]
	if timeRange == "" {
		timeRange = "1 hour"
	}

	dimensionCol := map[string]string{
		"query":    "query_hash",
		"database": "database_name",
		"login":    "login_name",
		"app":      "client_app",
	}[params.Dimension]
	if dimensionCol == "" {
		dimensionCol = "query_hash"
	}

	metricCol := map[string]string{
		"cpu":        "cpu_ms",
		"duration":   "duration_ms",
		"reads":      "logical_reads",
		"executions": "executions",
	}[params.Metric]
	if metricCol == "" {
		metricCol = "cpu_ms"
	}

	baseSelect := fmt.Sprintf(`
		SELECT %s AS dimension_value,
		       query_text,
		       MAX(database_name) AS database_name,
		       MAX(login_name) AS login_name,
		       MAX(client_app) AS client_app,
		       SUM(%s) AS metric_value,
		       SUM(executions) AS total_executions,
		       AVG(avg_cpu_ms) AS avg_cpu_ms,
		       AVG(avg_duration_ms) AS avg_duration_ms,
		       AVG(avg_reads) AS avg_reads
		FROM sqlserver_query_stats_interval
		WHERE server_instance_name = $1`,
		dimensionCol, metricCol)

	var rows pgx.Rows
	var err error
	if strings.TrimSpace(params.From) != "" && strings.TrimSpace(params.To) != "" {
		start, end, errParse := parseTimeRangeRFC3339(params.From, params.To)
		if errParse != nil {
			return nil, errParse
		}
		q := baseSelect + `
		  AND bucket_end >= $2
		  AND bucket_end <= $3
		GROUP BY ` + dimensionCol + `, query_text
		ORDER BY metric_value DESC
		LIMIT $4`
		rows, err = tl.pool.Query(ctx, q, params.InstanceName, start, end, params.Limit)
	} else {
		q := baseSelect + fmt.Sprintf(`
		  AND bucket_end > now() - INTERVAL '%s'
		GROUP BY %s, query_text
		ORDER BY metric_value DESC
		LIMIT $2`, timeRange, dimensionCol)
		rows, err = tl.pool.Query(ctx, q, params.InstanceName, params.Limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dimValue, queryText, dbName, loginName, clientApp sql.NullString
		var metricValue, totalExecutions, avgCPU, avgDuration, avgReads float64

		if err := rows.Scan(&dimValue, &queryText, &dbName, &loginName, &clientApp, &metricValue, &totalExecutions, &avgCPU, &avgDuration, &avgReads); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"dimension":         dimValue.String,
			"query_text":        queryText.String,
			"database_name":     dbName.String,
			"login_name":        loginName.String,
			"client_app":        clientApp.String,
			"metric_value":      metricValue,
			"total_executions":  totalExecutions,
			"avg_cpu_ms":        avgCPU,
			"avg_duration_ms":   avgDuration,
			"avg_reads":         avgReads,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetQueryStatsTimeSeries(ctx context.Context, instanceName, metric string, timeRange string) ([]map[string]interface{}, error) {
	tr := map[string]string{
		"15m": "15 minutes",
		"1h":  "1 hour",
		"24h": "24 hours",
	}[timeRange]
	if tr == "" {
		tr = "1 hour"
	}

	metricCol := map[string]string{
		"cpu":        "cpu_ms",
		"duration":   "duration_ms",
		"reads":      "logical_reads",
		"executions": "executions",
	}[metric]
	if metricCol == "" {
		metricCol = "cpu_ms"
	}

	query := fmt.Sprintf(`
		SELECT time_bucket('5 min', bucket_end) AS time,
		       SUM(%s) AS value
		FROM sqlserver_query_stats_interval
		WHERE server_instance_name = $1
		  AND bucket_end > now() - INTERVAL '%s'
		GROUP BY time
		ORDER BY time
	`, metricCol, tr)

	rows, err := tl.pool.Query(ctx, query, instanceName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var value float64

		if err := rows.Scan(&ts, &value); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"time":  ts,
			"value": value,
		})
	}
	return results, rows.Err()
}
