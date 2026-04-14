// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL blocking session detection and lock wait analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"log"
)

// CollectPgBlocking fetches PostgreSQL blocking queries
func (c *PgRepository) CollectPgBlocking(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			blocked.pid AS blocked_pid,
			blocked.query AS blocked_query,
			blocked.state AS blocked_state,
			blocked.duration AS blocked_duration_ms,
			blocking.pid AS blocking_pid,
			blocking.query AS blocking_query,
			blocking.state AS blocking_state
		FROM pg_stat_activity blocked
		JOIN pg_locks blocked_locks ON blocked.pid = blocked_locks.pid
		JOIN pg_locks blocking_locks ON blocked_locks.transactionid = blocking_locks.transactionid 
			AND blocked_locks.pid != blocking_locks.pid
		JOIN pg_stat_activity blocking ON blocking_locks.pid = blocking.pid
		WHERE blocked.state != 'idle'
		ORDER BY blocked.duration DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[PostgreSQL] Blocking Query Error: %v", err)
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

// CollectPgLocks fetches PostgreSQL lock statistics
func (c *PgRepository) CollectPgLocks(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			locktype,
			database::regclass::text AS relation,
			mode,
			GRANTED,
			COUNT(*) AS count
		FROM pg_locks
		WHERE database > 0
		GROUP BY locktype, relation, mode, granted
		ORDER BY count DESC
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
