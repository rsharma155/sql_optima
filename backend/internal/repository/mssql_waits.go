package repository

import (
	"database/sql"
	"log"
)

// CollectWaitStats returns current waits for user-database sessions only (Real-Time Diagnostics).
func (c *MssqlRepository) CollectWaitStats(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 50
			w.wait_type,
			COUNT(*) AS waiting_tasks_count,
			CAST(SUM(w.wait_duration_ms) AS FLOAT) AS wait_time_ms
		FROM sys.dm_os_waiting_tasks w
		INNER JOIN sys.dm_exec_sessions s ON w.session_id = s.session_id
		WHERE s.is_user_process = 1
		  AND s.database_id > 4
		  AND LOWER(ISNULL(DB_NAME(s.database_id), '')) <> 'distribution'
		  AND s.session_id > 50
		  AND w.wait_type NOT IN (N'CLR_SEMAPHORE', N'LAZYWRITER_SLEEP', N'RESOURCE_QUEUE', N'SLEEP_TASK', N'SLEEP_SYSTEMTASK', N'SQLTRACE_BUFFER_FLUSH', N'WAITFOR', N'XE_TIMER_EVENT', N'XE_DISPATCHER_WAIT')
		GROUP BY w.wait_type
		HAVING SUM(w.wait_duration_ms) > 0
		ORDER BY SUM(w.wait_duration_ms) DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] Wait Stats (RTD) Query Error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var waitType string
		var taskCount int64
		var waitMs float64
		if err := rows.Scan(&waitType, &taskCount, &waitMs); err == nil {
			results = append(results, map[string]interface{}{
				"wait_type":             waitType,
				"waiting_tasks_count":   taskCount,
				"wait_time_ms":          waitMs,
			})
		}
	}
	return results, nil
}

// CollectWaitingTasks fetches waiting tasks from sys.dm_os_waiting_tasks
func (c *MssqlRepository) CollectWaitingTasks(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 50
			t.session_id,
			t.wait_duration_ms,
			t.wait_type,
			t.resource_description
		FROM sys.dm_os_waiting_tasks t
		WHERE t.session_id > 50
		ORDER BY t.wait_duration_ms DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var sessionID int
		var waitDuration int64
		var waitType, resourceDesc string
		if err := rows.Scan(&sessionID, &waitDuration, &waitType, &resourceDesc); err == nil {
			results = append(results, map[string]interface{}{
				"session_id":           sessionID,
				"wait_duration_ms":     waitDuration,
				"wait_type":            waitType,
				"resource_description": resourceDesc,
			})
		}
	}
	return results, nil
}

// CollectLatchStats fetches latch statistics from sys.dm_os_latch_stats
func (c *MssqlRepository) CollectLatchStats(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 20 
			latch_class, 
			waiting_requests_count, 
			wait_time_ms 
		FROM sys.dm_os_latch_stats 
		WHERE waiting_requests_count > 0 
		ORDER BY wait_time_ms DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var resultsMap = make(map[string]interface{})
		columns, _ := rows.Columns()
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err == nil {
			for i, col := range columns {
				resultsMap[col] = values[i]
			}
			results = append(results, resultsMap)
		}
	}
	return results, nil
}
