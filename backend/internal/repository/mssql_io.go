// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server file I/O latency and throughput metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"log"
)

// CollectFileIOLatencyForRTD limits to user databases (database_id > 4) and excludes the replication distributor DB.
func (c *MssqlRepository) CollectFileIOLatencyForRTD(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			DB_NAME(mf.database_id) AS database_name,
			mf.name AS file_name,
			mf.type_desc AS file_type,
			CAST(vfs.num_of_reads AS BIGINT) AS num_of_reads,
			CAST(vfs.num_of_writes AS BIGINT) AS num_of_writes,
			CAST(vfs.io_stall_read_ms AS BIGINT) AS io_stall_read_ms,
			CAST(vfs.io_stall_write_ms AS BIGINT) AS io_stall_write_ms,
			CAST(vfs.io_stall_read_ms AS FLOAT) / NULLIF(vfs.num_of_reads, 0) AS read_latency_ms,
			CAST(vfs.io_stall_write_ms AS FLOAT) / NULLIF(vfs.num_of_writes, 0) AS write_latency_ms,
			CAST(vfs.io_stall_read_ms AS FLOAT) / NULLIF(vfs.num_of_reads, 0) AS avg_read_latency_ms,
			CAST(vfs.io_stall_write_ms AS FLOAT) / NULLIF(vfs.num_of_writes, 0) AS avg_write_latency_ms,
			mf.file_id AS file_id,
			mf.size * 8 / 1024 AS size_mb
		FROM sys.dm_io_virtual_file_stats(NULL, NULL) vfs
		INNER JOIN sys.master_files mf ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id
		WHERE mf.type_desc IN ('DATA', 'LOG')
		  AND vfs.database_id > 4
		  AND LOWER(ISNULL(DB_NAME(vfs.database_id), '')) <> 'distribution'
		ORDER BY database_name, file_type
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] File I/O Latency RTD Query Error: %v", err)
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

// CollectFileIOLatency fetches I/O latency from sys.dm_io_virtual_file_stats
func (c *MssqlRepository) CollectFileIOLatency(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			DB_NAME(mf.database_id) AS database_name,
			mf.name AS file_name,
			mf.type_desc AS file_type,
			CAST(vfs.num_of_reads AS BIGINT) AS num_of_reads,
			CAST(vfs.num_of_writes AS BIGINT) AS num_of_writes,
			CAST(vfs.io_stall_read_ms AS BIGINT) AS io_stall_read_ms,
			CAST(vfs.io_stall_write_ms AS BIGINT) AS io_stall_write_ms,
			CAST(vfs.io_stall_read_ms AS FLOAT) / NULLIF(vfs.num_of_reads, 0) AS read_latency_ms,
			CAST(vfs.io_stall_write_ms AS FLOAT) / NULLIF(vfs.num_of_writes, 0) AS write_latency_ms,
			CAST(vfs.io_stall_read_ms AS FLOAT) / NULLIF(vfs.num_of_reads, 0) AS avg_read_latency_ms,
			CAST(vfs.io_stall_write_ms AS FLOAT) / NULLIF(vfs.num_of_writes, 0) AS avg_write_latency_ms,
			mf.size * 8 / 1024 AS size_mb
		FROM sys.dm_io_virtual_file_stats(NULL, NULL) vfs
		INNER JOIN sys.master_files mf ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id
		WHERE mf.type_desc IN ('DATA', 'LOG')
		  AND vfs.database_id > 4
		  AND LOWER(ISNULL(DB_NAME(vfs.database_id), N'')) <> N'distribution'
		ORDER BY database_name, file_type
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] File I/O Latency Query Error: %v", err)
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

// CollectDiskUsage fetches disk usage from sys.master_files
func (c *MssqlRepository) CollectDiskUsage(db *sql.DB) (map[string]float64, error) {
	query := `
		SELECT 
			DB_NAME(database_id) AS database_name,
			CAST(SUM(size * 8 / 1024) AS FLOAT) AS size_mb
		FROM sys.master_files
		WHERE type_desc = 'DATA'
		GROUP BY database_id
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[MSSQL] Disk Usage Query Error: %v", err)
		return nil, err
	}
	defer rows.Close()

	diskUsage := make(map[string]float64)
	for rows.Next() {
		var dbName string
		var sizeMB float64
		if err := rows.Scan(&dbName, &sizeMB); err == nil {
			diskUsage[dbName] = sizeMB
		}
	}
	return diskUsage, nil
}
