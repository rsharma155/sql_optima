// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server blocking chain detection and lock analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"log"
	"strings"
)

// CollectBlockingChains returns aggregated blockers for Real-Time Diagnostics (user databases only; matches UI shape).
// If database is non-empty, scopes to that DB only.
func (c *MssqlRepository) CollectBlockingChains(db *sql.DB, database string) ([]map[string]interface{}, error) {
	query := `
		WITH BlockingTree AS (
			SELECT r.session_id AS Blocked_SPID,
				r.blocking_session_id AS Blocking_SPID,
				r.wait_time
			FROM sys.dm_exec_requests r
			INNER JOIN sys.dm_exec_sessions s ON r.session_id = s.session_id
			WHERE r.blocking_session_id > 0
			  AND r.session_id > 50
			  AND s.is_user_process = 1
			  AND s.database_id > 4
			  AND LOWER(ISNULL(DB_NAME(s.database_id), '')) <> 'distribution'
			  AND (@p1 = '' OR DB_NAME(s.database_id) = @p1)
		  AND LOWER(ISNULL(s.login_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		  AND LOWER(ISNULL(s.program_name, '')) NOT IN ('dbmonitor_user', 'go-mssqldb')
		)
		SELECT TOP 50
			b.Blocking_SPID AS Lead_Blocker,
			ISNULL(s.login_name, 'Unknown') AS Blocker_Login,
			ISNULL(s.program_name, 'Unknown') AS Blocker_App,
			COUNT(b.Blocked_SPID) AS Total_Victims,
			MAX(b.wait_time) AS Max_Wait_Time_ms
		FROM BlockingTree b
		INNER JOIN sys.dm_exec_sessions s ON b.Blocking_SPID = s.session_id
		GROUP BY b.Blocking_SPID, s.login_name, s.program_name
		ORDER BY Max_Wait_Time_ms DESC
	`

	rows, err := db.Query(query, strings.TrimSpace(database))
	if err != nil {
		log.Printf("[MSSQL] Blocking Query Error: %v", err)
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

// CollectLocks fetches lock statistics from sys.dm_tran_locks
func (c *MssqlRepository) CollectLocks(db *sql.DB) (int, int, map[string]int, error) {
	totalLocksQuery := `SELECT COUNT(*) FROM sys.dm_tran_locks WHERE request_session_id > 50`
	var totalLocks int
	if err := db.QueryRow(totalLocksQuery).Scan(&totalLocks); err != nil {
		log.Printf("[MSSQL] Total Locks Query Error: %v", err)
	}

	deadlockQuery := `
		WITH CTE AS (
			SELECT ROW_NUMBER() OVER(PARTITION BY blocked ORDER BY blocked DESC) AS rn, blocked, blocking
			FROM (
				SELECT blocked, 0 AS blocking FROM sys.dm_exec_requests WHERE blocked > 0
				UNION ALL
				SELECT 0, blocking_session_id FROM sys.dm_exec_requests WHERE blocking_session_id > 0
			) a(bblocked, bblocking)
		)
		SELECT COUNT(*) FROM CTE WHERE rn > 1
	`
	var deadlocks int
	if err := db.QueryRow(deadlockQuery).Scan(&deadlocks); err != nil {
		log.Printf("[MSSQL] Deadlock Query Error: %v", err)
	}

	locksByDB := make(map[string]int)
	dbLockQuery := `
		SELECT ISNULL(DB_NAME(resource_database_id), 'Unknown'), COUNT(*)
		FROM sys.dm_tran_locks
		WHERE request_session_id > 50
		GROUP BY resource_database_id
	`
	rows, err := db.Query(dbLockQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dbName string
			var count int
			if err := rows.Scan(&dbName, &count); err == nil {
				locksByDB[dbName] = count
			}
		}
	}

	return totalLocks, deadlocks, locksByDB, nil
}

// CollectSpinlockStats fetches spinlock statistics
func (c *MssqlRepository) CollectSpinlockStats(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT TOP 20 
			name AS spinlock_type, 
			collisions, 
			spins, 
			sleep_time AS sleep_time_ms,
			backoffs,
			spins_per_collision
		FROM sys.dm_os_spinlock_stats 
		ORDER BY spins DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var name, collisions, spins, sleep_time, backoffs, spins_per_collision interface{}
		if err := rows.Scan(&name, &collisions, &spins, &sleep_time, &backoffs, &spins_per_collision); err == nil {
			results = append(results, map[string]interface{}{
				"spinlock_type":       name,
				"collisions":          collisions,
				"spins":               spins,
				"sleep_time":          sleep_time,
				"backoffs":            backoffs,
				"spins_per_collision": spins_per_collision,
			})
		}
	}
	return results, nil
}
