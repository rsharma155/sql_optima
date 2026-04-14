// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Control Center metrics and health monitoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"database/sql"
	"fmt"
	"log"
)

// FetchWalBytesTotal returns cumulative wal_bytes from pg_stat_wal (PG 14+).
func (c *PgRepository) FetchWalBytesTotal(instanceName string) (uint64, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchWalBytesTotal: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var bytes sql.NullInt64
	err := db.QueryRow(`SELECT COALESCE(SUM(wal_bytes), 0) FROM pg_stat_wal`).Scan(&bytes)
	if err != nil {
		return 0, err
	}
	if !bytes.Valid || bytes.Int64 < 0 {
		return 0, nil
	}
	return uint64(bytes.Int64), nil
}

// FetchWalDirSizeMB returns total size of files in pg_wal directory (MB).
func (c *PgRepository) FetchWalDirSizeMB(instanceName string) (float64, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var mb float64
	err := db.QueryRow(`SELECT COALESCE(SUM(size), 0) / 1024.0 / 1024.0 FROM pg_ls_waldir()`).Scan(&mb)
	return mb, err
}

// FetchActiveWaitingSessions returns counts from pg_stat_activity.
func (c *PgRepository) FetchActiveWaitingSessions(instanceName string) (active int, waiting int, err error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, 0, fmt.Errorf("connection not found")
	}

	err = db.QueryRow(`
		SELECT 
			COUNT(*) FILTER (WHERE state = 'active') AS active,
			COUNT(*) FILTER (WHERE wait_event IS NOT NULL) AS waiting
		FROM pg_stat_activity
	`).Scan(&active, &waiting)
	return active, waiting, err
}

// FetchSlowQueriesCount returns number of queries in pg_stat_statements with mean_exec_time > thresholdMs.
func (c *PgRepository) FetchSlowQueriesCount(instanceName string, thresholdMs float64) (int, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}
	var cnt int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_stat_statements WHERE mean_exec_time > $1`, thresholdMs).Scan(&cnt)
	return cnt, err
}

func (c *PgRepository) FetchBlockingSessionsCount(instanceName string) (int, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var cnt int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity WHERE wait_event_type='Lock'`).Scan(&cnt)
	return cnt, err
}

func (c *PgRepository) FetchAutovacuumWorkers(instanceName string) (int, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var cnt int
	err := db.QueryRow(`SELECT COUNT(*) FROM pg_stat_activity WHERE query ILIKE '%autovacuum%'`).Scan(&cnt)
	return cnt, err
}

func (c *PgRepository) FetchDeadTupleRatioPct(instanceName string) (float64, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var pct float64
	err := db.QueryRow(`
		SELECT
			CASE WHEN (SUM(n_dead_tup) + SUM(n_live_tup)) > 0
				THEN (SUM(n_dead_tup)::float / (SUM(n_dead_tup) + SUM(n_live_tup))) * 100.0
				ELSE 0
			END
		FROM pg_stat_user_tables
	`).Scan(&pct)
	return pct, err
}

func (c *PgRepository) FetchCacheHitRatioPct(instanceName string) (float64, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
		}
	}
	if !ok || db == nil {
		return 0, fmt.Errorf("connection not found")
	}

	var pct float64
	err := db.QueryRow(`
		SELECT
			CASE WHEN (SUM(blks_hit) + SUM(blks_read)) > 0
				THEN (SUM(blks_hit)::float * 100.0) / (SUM(blks_hit) + SUM(blks_read))
				ELSE 0
			END
		FROM pg_stat_database
		WHERE datname IS NOT NULL
		  AND datname NOT LIKE 'template%';
	`).Scan(&pct)
	return pct, err
}

func retainedWalMBFromBytes(b int64) float64 {
	if b <= 0 {
		return 0
	}
	return float64(b) / 1024.0 / 1024.0
}

