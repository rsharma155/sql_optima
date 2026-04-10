package collectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectBlockingLocks returns active blocking tree and "Idle in Transaction" queries
// Parameters: ctx context.Context, db *sql.DB
// Returns: []BlockingNode, error
func CollectBlockingLocks(ctx context.Context, db *sql.DB) ([]models.BlockingNode, error) {
	query := `
		SELECT TOP 50
			r.session_id,
			r.blocking_session_id,
			ISNULL(s.login_name, 'Unknown') AS login_name,
			ISNULL(s.host_name, 'Unknown') AS host_name,
			ISNULL(s.program_name, 'Unknown') AS program_name,
			ISNULL(DB_NAME(r.database_id), 'Unknown') AS database_name,
			ISNULL(LEFT(t.text, 2000), 'N/A') AS query_text,
			r.status,
			r.command,
			ISNULL(r.wait_type, 'NONE') AS wait_type,
			r.wait_time AS wait_time_ms,
			r.cpu_time AS cpu_time_ms,
			r.total_elapsed_time AS total_elapsed_time_ms,
			r.row_count,
			0 AS level
		FROM sys.dm_exec_requests r
		INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
		CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) t
		WHERE r.session_id > 50
		  AND r.blocking_session_id > 0
		UNION ALL
		SELECT TOP 50
			s.session_id,
			0 AS blocking_session_id,
			s.login_name,
			s.host_name,
			s.program_name,
			DB_NAME(s.database_id) AS database_name,
			'Idle in Transaction' AS query_text,
			s.status,
			'' AS command,
			'' AS wait_type,
			0 AS wait_time_ms,
			0 AS cpu_time_ms,
			0 AS total_elapsed_time_ms,
			0 AS row_count,
			0 AS level
		FROM sys.dm_exec_sessions s
		WHERE s.status = 'idle_in_transaction'
		  AND s.session_id > 50
		  AND s.session_id <> @@SPID
		ORDER BY level DESC, wait_time_ms DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Collector] CollectBlockingLocks Error: %v", err)
		return nil, fmt.Errorf("failed to fetch blocking locks: %w", err)
	}
	defer rows.Close()

	var results []models.BlockingNode
	for rows.Next() {
		var b models.BlockingNode
		var cpuTime, totalElapsed, waitTime, rowCount sql.NullInt64

		err := rows.Scan(
			&b.SessionID, &b.BlockingSessionID, &b.LoginName, &b.HostName,
			&b.ProgramName, &b.DatabaseName, &b.QueryText, &b.Status,
			&b.Command, &b.WaitType, &waitTime, &cpuTime, &totalElapsed,
			&rowCount, &b.Level,
		)
		if err != nil {
			log.Printf("[Collector] CollectBlockingLocks Scan Error: %v", err)
			continue
		}

		if cpuTime.Valid {
			b.CPUTimeMs = cpuTime.Int64
		}
		if totalElapsed.Valid {
			b.TotalElapsedTimeMs = totalElapsed.Int64
		}
		if waitTime.Valid {
			b.WaitTimeMs = waitTime.Int64
		}
		if rowCount.Valid {
			b.RowCount = rowCount.Int64
		}

		results = append(results, b)
	}

	return results, rows.Err()
}
