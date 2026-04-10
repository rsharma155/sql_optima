package collectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectWaitStats returns wait statistics from sys.dm_os_wait_stats
// Parameters: ctx context.Context, db *sql.DB
// Returns: []WaitStat, error
func CollectWaitStats(ctx context.Context, db *sql.DB) ([]models.WaitStat, error) {
	query := `
		SELECT wait_type, CAST(wait_time_ms AS FLOAT), CAST(waiting_tasks_count AS BIGINT)
		FROM sys.dm_os_wait_stats WITH (NOLOCK) 
		WHERE wait_type NOT IN (
			'DIRTY_PAGE_POLL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 
			'LAZYWRITER_SLEEP', 'LOGMGR_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 
			'XE_DISPATCHER_WAIT', 'XE_TIMER_EVENT', 'SQLTRACE_BUFFER_FLUSH', 
			'SLEEP_TASK', 'BROKER_TO_FLUSH', 'SP_SERVER_DIAGNOSTICS_SLEEP'
		) 
		AND wait_time_ms > 0
		ORDER BY wait_time_ms DESC`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[Collector] CollectWaitStats Error: %v", err)
		return nil, fmt.Errorf("failed to fetch wait stats: %w", err)
	}
	defer rows.Close()

	var results []models.WaitStat
	for rows.Next() {
		var w models.WaitStat
		var waitTime sql.NullFloat64
		var waitingTasks sql.NullInt64

		err := rows.Scan(&w.WaitType, &waitTime, &waitingTasks)
		if err != nil {
			log.Printf("[Collector] CollectWaitStats Scan Error: %v", err)
			continue
		}

		if waitTime.Valid {
			w.WaitTimeMs = waitTime.Float64
		}
		if waitingTasks.Valid {
			w.WaitingTasks = waitingTasks.Int64
		}

		results = append(results, w)
	}

	return results, rows.Err()
}
