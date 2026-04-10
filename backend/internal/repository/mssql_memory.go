package repository

import (
	"database/sql"
	"log"
)

// CollectMemoryMetrics fetches memory statistics from sys.dm_os_performance_counters and sys.dm_os_sys_memory
func (c *MssqlRepository) CollectMemoryMetrics(db *sql.DB) (float64, error) {
	// Page Life Expectancy (PLE)
	pleQuery := `SELECT cntr_value FROM sys.dm_os_performance_counters WHERE counter_name = 'Page life expectancy'`
	var ple float64
	if err := db.QueryRow(pleQuery).Scan(&ple); err != nil {
		log.Printf("[MSSQL] PLE Query Error: %v", err)
	}

	// Buffer Pool Size
	bufQuery := `SELECT cntr_value / 1024 FROM sys.dm_os_performance_counters WHERE counter_name = 'Buffer Pool Size (KB)'`
	var bufPoolSize float64
	if err := db.QueryRow(bufQuery).Scan(&bufPoolSize); err != nil {
		log.Printf("[MSSQL] Buffer Pool Query Error: %v", err)
	}

	// Memory Clerk Count
	clerkQuery := `SELECT COUNT(DISTINCT memory_clerk_address) FROM sys.dm_os_memory_clerks`
	var clerkCount int
	if err := db.QueryRow(clerkQuery).Scan(&clerkCount); err != nil {
		log.Printf("[MSSQL] Memory Clerk Query Error: %v", err)
	}

	// Calculate memory usage percentage
	var memUsage float64 = 0
	memQuery := `
		SELECT 
			(CAST(total_physical_memory_kb AS FLOAT) - CAST(available_physical_memory_kb AS FLOAT)) / 
			CAST(total_physical_memory_kb AS FLOAT) * 100
		FROM sys.dm_os_sys_memory
	`
	if err := db.QueryRow(memQuery).Scan(&memUsage); err != nil {
		log.Printf("[MSSQL] Memory Usage Query Error: %v", err)
	}

	return memUsage, nil
}

// CollectMemoryClerks fetches memory clerk information
func (c *MssqlRepository) CollectMemoryClerks(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			type AS clerk_type,
			memory_node_id AS memory_node,
			CAST(SUM(pages_kb) / 1024.0 AS FLOAT) AS pages_mb,
			CAST(SUM(virtual_memory_reserved_kb) / 1024.0 AS FLOAT) AS virtual_memory_reserved_mb,
			CAST(SUM(virtual_memory_committed_kb) / 1024.0 AS FLOAT) AS virtual_memory_committed_mb,
			CAST(SUM(awe_allocated_kb) / 1024.0 AS FLOAT) AS awe_memory_mb
		FROM sys.dm_os_memory_clerks
		GROUP BY type, memory_node_id
		HAVING SUM(pages_kb) > 1024
		ORDER BY pages_mb DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var clerkType string
		var node int64
		var pagesMB, rsvMB, comMB, aweMB float64
		if err := rows.Scan(&clerkType, &node, &pagesMB, &rsvMB, &comMB, &aweMB); err == nil {
			results = append(results, map[string]interface{}{
				"clerk_type":                 clerkType,
				"memory_node":                node,
				"pages_mb":                   pagesMB,
				"virtual_memory_reserved_mb": rsvMB,
				"virtual_memory_committed_mb": comMB,
				"awe_memory_mb":              aweMB,
			})
		}
	}
	return results, nil
}

// CollectMemoryGrants fetches memory grant information
func (c *MssqlRepository) CollectMemoryGrants(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 20
			session_id,
			request_id,
			grant_time,
			requested_memory_kb,
			granted_memory_kb,
			ideal_memory_kb,
			max_used_memory_kb,
			queue_id,
			wait_order,
			is_next_candidate
		FROM sys.dm_exec_query_memory_grants
		WHERE granted_memory_kb > 0
		ORDER BY granted_memory_kb DESC
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
