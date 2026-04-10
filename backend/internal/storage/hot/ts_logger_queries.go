package hot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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

	for _, q := range queries {
		queryHash := fmt.Sprintf("%v", q["Query_Hash"])
		execCount := getInt64(q, "Executions")
		cpuMs := getFloat64(q, "Total_CPU_ms")
		logicalReads := getInt64(q, "Total_Logical_Reads")

		if prev, exists := tl.prevTopQueries[queryHash]; exists {
			execDelta := execCount - prev["executions"]

			if execDelta > 0 {
				insertQuery := `INSERT INTO sqlserver_top_queries (capture_timestamp, server_instance_name, query_hash, query_text, execution_count, avg_cpu_ms, total_cpu_ms, avg_logical_reads, total_logical_reads, exec_time_ms)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
					ON CONFLICT (server_instance_name, query_hash, capture_timestamp) DO UPDATE SET
						execution_count = EXCLUDED.execution_count,
						avg_cpu_ms = EXCLUDED.avg_cpu_ms,
						total_cpu_ms = EXCLUDED.total_cpu_ms,
						avg_logical_reads = EXCLUDED.avg_logical_reads,
						total_logical_reads = EXCLUDED.total_logical_reads,
						exec_time_ms = EXCLUDED.exec_time_ms`
				_, err := tl.pool.Exec(ctx, insertQuery,
					now, instanceName, queryHash, getStr(q, "Query_Text"),
					execDelta, cpuMs/float64(execDelta), cpuMs, float64(logicalReads)/float64(execDelta), logicalReads, cpuMs/float64(execDelta),
				)
				if err != nil {
					log.Printf("[TSLogger] Failed to log top query: %v", err)
				}
			}
		}

		tl.prevTopQueries[queryHash] = map[string]int64{
			"executions": execCount,
			"cpu":        int64(cpuMs),
			"reads":      logicalReads,
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
		       avg_cpu_ms, total_cpu_ms, avg_logical_reads, total_logical_reads
		FROM sqlserver_top_queries
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC, total_cpu_ms DESC
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
		var execCount int64
		var avgCpuMs, totalCpuMs, avgLogicalReads, totalLogicalReads float64

		if err := rows.Scan(&ts, &queryHash, &queryText, &execCount, &avgCpuMs, &totalCpuMs, &avgLogicalReads, &totalLogicalReads); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":           ts,
			"query_hash":          queryHash,
			"query_text":          queryText.String,
			"execution_count":     execCount,
			"avg_cpu_ms":          avgCpuMs,
			"total_cpu_ms":        totalCpuMs,
			"avg_logical_reads":   avgLogicalReads,
			"total_logical_reads": totalLogicalReads,
		})
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerTopQueriesWithRange(ctx context.Context, instanceName string, limit int, from, to string) ([]map[string]interface{}, error) {
	start, end, err := parseTimeRange(from, to)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
		       query_hash,
		       SUM(execution_count) AS total_executions,
		       AVG(cpu_time_ms) AS avg_cpu_ms,
		       SUM(cpu_time_ms) AS total_cpu_ms,
		       AVG(logical_reads) AS avg_logical_reads,
		       SUM(logical_reads) AS total_logical_reads
		FROM sqlserver_top_queries
		WHERE server_instance_name = $1
		  AND capture_timestamp >= $2
		  AND capture_timestamp <= $3
		GROUP BY bucket, query_hash
		ORDER BY bucket DESC, total_cpu_ms DESC
		LIMIT $4
	`

	rows, err := tl.pool.Query(ctx, query, instanceName, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var queryHash string
		var totalExecutions int64
		var avgCpuMs, totalCpuMs, avgLogicalReads, totalLogicalReads float64

		if err := rows.Scan(&bucket, &queryHash, &totalExecutions, &avgCpuMs, &totalCpuMs, &avgLogicalReads, &totalLogicalReads); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"timestamp":           bucket,
			"query_hash":          queryHash,
			"execution_count":     totalExecutions,
			"avg_cpu_ms":          avgCpuMs,
			"total_cpu_ms":        totalCpuMs,
			"avg_logical_reads":   avgLogicalReads,
			"total_logical_reads": totalLogicalReads,
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

		tl.prevLongRunningHash[key] = int64(q.TotalElapsedTimeMs)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(queries); i++ {
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
		batch.Queue(`INSERT INTO query_store_stats (
			capture_timestamp, server_name, database_name,
			query_hash, query_text, executions,
			avg_duration_ms, avg_cpu_ms, avg_logical_reads, total_cpu_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			now, r.ServerName, r.DatabaseName,
			r.QueryHash, r.QueryText, r.Executions,
			r.AvgDurationMs, r.AvgCpuMs, r.AvgLogicalReads, r.TotalCpuMs,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			log.Printf("[TSLogger] Failed to execute batch for query store stats: %v", err)
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
