// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server database throughput statistics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"log"
)

type DatabaseThroughputStats struct {
	DatabaseName        string
	UserSeeks           int64
	UserScans           int64
	UserLookups         int64
	UserWrites          int64
	TotalReads         int64
	TotalWrites        int64
	TPS                float64
	BatchRequestsPerSec float64
}

func (c *SqlServerRepository) FetchDatabaseThroughput(instanceName string) ([]DatabaseThroughputStats, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}

	query := `
		SELECT /* SQL_OPTIMA */   
			DB_NAME(s.database_id) AS database_name,
			ISNULL(SUM(s.user_seeks), 0) AS idx_seeks,
			ISNULL(SUM(s.user_scans), 0) AS idx_scans,
			ISNULL(SUM(s.user_lookups), 0) AS idx_lookups,
			ISNULL(SUM(s.user_updates), 0) AS idx_updates,
			ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups), 0) AS total_idx_reads,
			ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups + s.user_updates), 0) AS total_idx_activity
		FROM sys.dm_db_index_usage_stats s
		WHERE s.database_id > 4
		GROUP BY s.database_id
		HAVING ISNULL(SUM(s.user_seeks + s.user_scans + s.user_lookups + s.user_updates), 0) > 0
		ORDER BY total_idx_activity DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[SQLSERVER] FetchDatabaseThroughput Error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	var results []DatabaseThroughputStats
	for rows.Next() {
		var s DatabaseThroughputStats
		var idxSeeks, idxScans, idxLookups, idxUpdates, totalReads, totalActivity int64
		if err := rows.Scan(&s.DatabaseName, &idxSeeks, &idxScans, &idxLookups, &idxUpdates, &totalReads, &totalActivity); err != nil {
			log.Printf("[SQLSERVER] FetchDatabaseThroughput Scan Error: %v", err)
			continue
		}
		s.UserSeeks = idxSeeks
		s.UserScans = idxScans
		s.UserLookups = idxLookups
		s.UserWrites = idxUpdates
		s.TotalReads = totalReads
		s.TotalWrites = idxUpdates
		s.TPS = float64(totalActivity) / 60.0
		results = append(results, s)
	}

	batchQuery := `
		SELECT /* SQL_OPTIMA */   
			DB_NAME(r.database_id) AS database_name,
			COUNT(*) AS batch_count
		FROM sys.dm_exec_requests r
		WHERE r.database_id > 4
		GROUP BY r.database_id
	`

	batchRows, batchErr := db.Query(batchQuery)
	if batchErr == nil {
		defer batchRows.Close()
		batchMap := make(map[string]float64)
		for batchRows.Next() {
			var dbName string
			var bps float64
			if err := batchRows.Scan(&dbName, &bps); err == nil {
				batchMap[dbName] = bps
			}
		}
		for i := range results {
			if bps, ok := batchMap[results[i].DatabaseName]; ok {
				results[i].BatchRequestsPerSec = bps
			}
		}
	}

	return results, rows.Err()
}