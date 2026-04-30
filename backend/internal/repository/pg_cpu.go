// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL CPU and system resource metrics collection from pg_stat_database and system views.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"log"
)

// CollectPgSystemStats fetches PostgreSQL system stats (CPU and Memory)
func (c *PgRepository) CollectPgSystemStats(db *sql.DB) (float64, float64, error) {
	// First check if the extension functions exist to avoid query planning errors
	var hasProctab bool
	_ = db.QueryRow("/* SQL_OPTIMA */   SELECT EXISTS (SELECT 1 FROM pg_proc WHERE proname = 'pg_stat_cpu')").Scan(&hasProctab)

	if !hasProctab {
		// If no pg_proctab, return zeros (or 0.1 to avoid being squashed by hash check if needed)
		// Better to just return 0 and let the caller/collector handle it.
		return 0, 0, nil
	}

	query := `
			/* SQL_OPTIMA */   	
			SELECT 
				(SELECT   COALESCE(avg(percent), 0) FROM (
					SELECT    100 * (1 - (avail::float / total::float)) AS percent
					FROM (
						SELECT    available_memory AS avail, total_memory AS total FROM pg_os_info()
					) mem
				) m) AS memory_usage,
			(SELECT   COALESCE(util, 0) FROM pg_stat_cpu() WHERE util IS NOT NULL LIMIT 1) AS cpu_usage
	`

	var cpuUsage, memUsage float64
	err := db.QueryRow(query).Scan(&memUsage, &cpuUsage)
	if err != nil {
		log.Printf("[PostgreSQL] System Stats Query Error (despite proc check): %v", err)
		return 0, 0, nil
	}

	return cpuUsage, memUsage, nil
}

// CollectPgServerInfo fetches PostgreSQL version and uptime
func (c *PgRepository) CollectPgServerInfo(db *sql.DB) (string, string, error) {
	query := `
		/* SQL_OPTIMA */
		SELECT /* SQL_OPTIMA */   
			version() AS version,
			date_trunc('second', now() - pg_postmaster_start_time())::text AS uptime
		FROM pg_postmaster_start_time()
	`

	var version, uptime string
	err := db.QueryRow(query).Scan(&version, &uptime)
	if err != nil {
		log.Printf("[PostgreSQL] Server Info Query Error: %v", err)
	}
	return version, uptime, err
}
