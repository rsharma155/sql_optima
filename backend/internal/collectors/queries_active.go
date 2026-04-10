package collectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectActiveQueries returns currently executing queries (Top 50 live queries)
// Parameters: ctx context.Context, db *sql.DB
// Returns: []ActiveQuery, error
func CollectActiveQueries(ctx context.Context, db *sql.DB) ([]models.ActiveQuery, error) {
	query := `
		SELECT TOP 50
			r.session_id,
			r.request_id,
			ISNULL(DB_NAME(r.database_id), 'Unknown') AS database_name,
			ISNULL(s.login_name, 'Unknown') AS login_name,
			ISNULL(s.host_name, 'Unknown') AS host_name,
			ISNULL(s.program_name, 'Unknown') AS program_name,
			ISNULL(LEFT(t.text, 2000), 'N/A') AS query_text,
			r.status,
			r.command,
			ISNULL(r.wait_type, 'RUNNING') AS wait_type,
			r.wait_time AS wait_time_ms,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.reads,
			r.writes,
			ISNULL(r.granted_query_memory * 8 / 1024, 0) AS granted_query_memory_mb,
			r.row_count,
			ISNULL(r.percent_complete, '0') AS percent_complete
		FROM sys.dm_exec_requests r
		INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE r.session_id > 50
		  AND r.session_id <> @@SPID
		  AND r.status IN ('running', 'runnable', 'suspended')
		ORDER BY r.cpu_time DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Collector] CollectActiveQueries Error: %v", err)
		return nil, fmt.Errorf("failed to fetch active queries: %w", err)
	}
	defer rows.Close()

	var results []models.ActiveQuery
	for rows.Next() {
		var q models.ActiveQuery
		var cpuTime, totalElapsed, reads, writes, grantedMemory, rowCount sql.NullInt64
		var waitTime sql.NullInt64

		err := rows.Scan(
			&q.SessionID, &q.RequestID, &q.DatabaseName, &q.LoginName,
			&q.HostName, &q.ProgramName, &q.QueryText, &q.Status,
			&q.Command, &q.WaitType, &waitTime, &cpuTime, &totalElapsed,
			&reads, &writes, &grantedMemory, &rowCount, &q.PercentComplete,
		)
		if err != nil {
			log.Printf("[Collector] CollectActiveQueries Scan Error: %v", err)
			continue
		}

		if cpuTime.Valid {
			q.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			q.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if waitTime.Valid {
			q.WaitTimeMs = waitTime.Int64
		}
		if reads.Valid {
			q.Reads = reads.Int64
		}
		if writes.Valid {
			q.Writes = writes.Int64
		}
		if grantedMemory.Valid {
			q.GrantedMemoryMB = int(grantedMemory.Int64)
		}
		if rowCount.Valid {
			q.RowCount = rowCount.Int64
		}

		results = append(results, q)
	}

	return results, rows.Err()
}

// CollectLongRunningQueries returns queries running longer than threshold
// Uses sys.dm_exec_input_buffer for additional safety
// Parameters: ctx context.Context, db *sql.DB
// Returns: []LongRunningQuery, error
func CollectLongRunningQueries(ctx context.Context, db *sql.DB) ([]models.LongRunningQuery, error) {
	query := `
		SELECT TOP 50
			r.session_id,
			r.request_id,
			DB_NAME(r.database_id) AS database_name,
			s.login_name,
			s.host_name,
			s.program_name,
			CASE 
				WHEN qp.objectid IS NOT NULL 
				THEN QUOTENAME(OBJECT_SCHEMA_NAME(qp.objectid, r.database_id)) 
					 + '.' + QUOTENAME(OBJECT_NAME(qp.objectid, r.database_id))
				ELSE
					SUBSTRING(
						qt.text,
						(r.statement_start_offset/2) + 1,
						(
							CASE r.statement_end_offset
								WHEN -1 THEN DATALENGTH(qt.text)
								ELSE r.statement_end_offset
							END - r.statement_start_offset
						) / 2 + 1
					)
			END AS query_text,
			r.wait_type,
			r.blocking_session_id,
			r.status,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.reads,
			r.writes,
			(r.granted_query_memory * 8) / 1024 AS granted_query_memory_mb,
			r.row_count
		FROM sys.dm_exec_requests r
		JOIN sys.dm_exec_sessions s 
			ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) qt
		OUTER APPLY sys.dm_exec_query_plan(r.plan_handle) qp
		WHERE r.session_id <> @@SPID AND r.session_id > 50
		AND r.total_elapsed_time >= 5000
		AND s.is_user_process = 1
		ORDER BY r.total_elapsed_time DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Collector] CollectLongRunningQueries Error: %v", err)
		return nil, fmt.Errorf("failed to fetch long running queries: %w", err)
	}
	defer rows.Close()

	var results []models.LongRunningQuery
	for rows.Next() {
		var q models.LongRunningQuery
		var cpuTime, totalElapsed, reads, writes, grantedMemory, rowCount sql.NullInt64
		var blockingSessionID sql.NullInt64

		err := rows.Scan(
			&q.SessionID, &q.RequestID, &q.DatabaseName, &q.LoginName,
			&q.HostName, &q.ProgramName, &q.QueryText, &q.WaitType,
			&blockingSessionID, &q.Status, &cpuTime, &totalElapsed,
			&reads, &writes, &grantedMemory, &rowCount,
		)
		if err != nil {
			log.Printf("[Collector] CollectLongRunningQueries Scan Error: %v", err)
			continue
		}

		if cpuTime.Valid {
			q.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			q.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if reads.Valid {
			q.Reads = reads.Int64
		}
		if writes.Valid {
			q.Writes = writes.Int64
		}
		if blockingSessionID.Valid {
			q.BlockingSessionID = int(blockingSessionID.Int64)
		}
		if grantedMemory.Valid {
			q.GrantedQueryMemoryMB = int(grantedMemory.Int64)
		}
		if rowCount.Valid {
			q.RowCount = rowCount.Int64
		}

		results = append(results, q)
	}

	return results, rows.Err()
}
