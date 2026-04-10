package repository

import (
	"database/sql"
	"log"
)

// CollectWaitStats fetches wait statistics from sys.dm_os_wait_stats
func (c *MssqlRepository) CollectWaitStats(db *sql.DB) ([]map[string]interface{}, error) {
	query := `SELECT wait_type, CAST(wait_time_ms AS FLOAT) FROM sys.dm_os_wait_stats WITH (NOLOCK) WHERE wait_type NOT IN ('DIRTY_PAGE_POLL', 'HADR_FILESTREAM_IOMGR_IOCOMPLETION', 'LAZYWRITER_SLEEP', 'LOGMGR_QUEUE', 'REQUEST_FOR_DEADLOCK_SEARCH', 'XE_DISPATCHER_WAIT', 'XE_TIMER_EVENT', 'SQLTRACE_BUFFER_FLUSH', 'SLEEP_TASK', 'BROKER_TO_FLUSH', 'SP_SERVER_DIAGNOSTICS_SLEEP') AND wait_time_ms > 0`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] Wait Stats Query Error: %v", err)
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var waitType string
		var waitTime sql.NullFloat64
		if err := rows.Scan(&waitType, &waitTime); err == nil {
			results = append(results, map[string]interface{}{
				"wait_type":    waitType,
				"wait_time_ms": waitTime.Float64,
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
