package repository

import (
	"database/sql"
	"log"
)

// CollectDatabaseThroughput fetches database throughput metrics
func (c *MssqlRepository) CollectDatabaseThroughput(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			DB_NAME(database_id) AS database_name,
			CAST(SUM(num_reads) AS BIGINT) AS total_reads,
			CAST(SUM(num_writes) AS BIGINT) AS total_writes,
			CAST(SUM(num_reads + num_writes) AS BIGINT) AS total_io,
			CAST(SUM(io_stall_read_ms) AS BIGINT) AS total_read_stall_ms,
			CAST(SUM(io_stall_write_ms) AS BIGINT) AS total_write_stall_ms
		FROM sys.dm_io_virtual_file_stats(NULL, NULL)
		WHERE database_id > 4
		GROUP BY database_id
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] Database Throughput Query Error: %v", err)
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

// CollectConnectionStats fetches connection statistics by application (Real-Time Diagnostics: user DB workloads only).
func (c *MssqlRepository) CollectConnectionStats(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 20
			ISNULL(program_name, 'Unknown') AS program_name,
			COUNT(*) AS connection_count,
			COUNT(DISTINCT login_name) AS unique_logins
		FROM sys.dm_exec_sessions s
		WHERE s.is_user_process = 1
		  AND s.database_id > 4
		  AND LOWER(ISNULL(DB_NAME(s.database_id), '')) <> 'distribution'
		  AND s.login_name NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND s.program_name NOT IN ('dbmonitor_user', 'go-mssqldb')
		GROUP BY program_name
		ORDER BY connection_count DESC
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
