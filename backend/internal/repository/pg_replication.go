// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL replication monitoring including lag, slot status, and streaming health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"log"
)

// CollectPgReplication fetches PostgreSQL replication stats
func (c *PgRepository) CollectPgReplication(db *sql.DB) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			pid,
			usesysid,
			usename,
			application_name,
			client_addr,
			backend_start,
			backend_xmin,
			state,
			sent_lsn,
			write_lsn,
			flush_lsn,
			replay_lsn,
			COALESCE(write_lag, INTERVAL '0') AS write_lag,
			COALESCE(flush_lag, INTERVAL '0') AS flush_lag,
			COALESCE(replay_lag, INTERVAL '0') AS replay_lag
		FROM pg_stat_replication
		ORDER BY state, backend_start DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[PostgreSQL] Replication Query Error: %v", err)
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

// CollectPgReplicationLag fetches replication lag in MB
func (c *PgRepository) CollectPgReplicationLag(db *sql.DB) (float64, string, error) {
	query := `
		SELECT 
			COALESCE(EXTRACT(EPOCH FROM (now() - replay_lag)) * 1024, 0) AS lag_mb,
			COALESCE(state, 'unknown') AS state
		FROM pg_stat_replication
		ORDER BY replay_lag DESC
		LIMIT 1
	`

	var lagMB float64
	var state string
	err := db.QueryRow(query).Scan(&lagMB, &state)
	if err != nil {
		log.Printf("[PostgreSQL] Replication Lag Query Error: %v", err)
		return 0, "none", err
	}
	return lagMB, state, nil
}
