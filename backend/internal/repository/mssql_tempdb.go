package repository

import (
	"database/sql"
	"log"
)

// CollectTempDBUsage fetches TempDB usage statistics
func (c *MssqlRepository) CollectTempDBUsage(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			ISNULL(DB_NAME(database_id), 'tempdb') AS database_name,
			file_id,
			type_desc,
			size * 8 / 1024 AS size_mb,
			CAST(size * 8.0 / 1024 AS FLOAT) - CAST(FILEPROPERTY(name, 'SpaceUsed') * 8.0 / 1024 AS FLOAT) AS free_space_mb
		FROM sys.master_files
		WHERE database_id = 2
		ORDER BY file_id
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] TempDB Usage Query Error: %v", err)
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

// CollectTempDBStats fetches detailed TempDB statistics
func (c *MssqlRepository) CollectTempDBStats(db *sql.DB) ([]map[string]interface{}, error) {
	// Query 1: TempDB file usage
	fileQuery := `
		SELECT 
			DB_NAME() AS database_name,
			t.name AS file_name,
			t.type_desc AS file_type,
			t.size_mb AS allocated_mb,
			FILEPROPERTY(t.name, 'SpaceUsed') AS used_mb,
			(t.size_mb - FILEPROPERTY(t.name, 'SpaceUsed')) AS free_mb,
			t.max_size AS max_size_mb,
			t.growth AS growth_mb
		FROM tempdb.sys.database_files t
		ORDER BY t.type
	`

	fileRows, err := db.Query(fileQuery)
	if err != nil {
		return nil, err
	}
	defer fileRows.Close()

	var results []map[string]interface{}
	for fileRows.Next() {
		var dbName, fileName, fileType interface{}
		var allocated, used, free, maxSize, growth interface{}
		if err := fileRows.Scan(&dbName, &fileName, &fileType, &allocated, &used, &free, &maxSize, &growth); err == nil {
			results = append(results, map[string]interface{}{
				"database_name": dbName,
				"file_name":     fileName,
				"file_type":     fileType,
				"allocated_mb":  allocated,
				"used_mb":       used,
				"free_mb":       free,
				"max_size_mb":   maxSize,
				"growth_mb":     growth,
			})
		}
	}

	// If no results from files, query for active tempdb requests
	if len(results) == 0 {
		requestQuery := `
			SELECT TOP 20
				DB_NAME() AS database_name,
				s.session_id,
				s.request_id,
				s.requested_memory_kb / 1024.0 AS requested_memory_mb,
				s.granted_memory_kb / 1024.0 AS granted_memory_mb,
				s.used_memory_kb / 1024.0 AS used_memory_mb,
				s.query_cost
			FROM sys.dm_exec_query_memory_grants s
			WHERE s.granted_memory_kb > 0
			ORDER BY s.granted_memory_kb DESC
		`

		requestRows, err := db.Query(requestQuery)
		if err != nil {
			return nil, err
		}
		defer requestRows.Close()

		for requestRows.Next() {
			var dbName interface{}
			var sessionId, requestId, requested, granted, used, queryCost interface{}
			if err := requestRows.Scan(&dbName, &sessionId, &requestId, &requested, &granted, &used, &queryCost); err == nil {
				results = append(results, map[string]interface{}{
					"database_name": dbName,
					"session_id":    sessionId,
					"request_id":    requestId,
					"requested_mb":  requested,
					"granted_mb":    granted,
					"used_mb":       used,
					"query_cost":    queryCost,
				})
			}
		}
	}

	return results, nil
}
