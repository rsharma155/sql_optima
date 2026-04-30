// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL DBA observation metrics (XID, bloat, hit rates).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"log"
)

// DBObservationMetrics holds critical DBA-focused health metrics for PostgreSQL
type DBObservationMetrics struct {
	XIDAge               int64   // max age(datfrozenxid) across user DBs
	XIDWraparoundPct     float64 // Percentage toward autovacuum_freeze_max_age (0-100+)
	IndexHitPct          float64 // Index lookup vs sequential scan ratio (0-100)
	IdleInTransactionCnt int     // Count of dangerous idle-in-transaction connections
	WALFails             int64   // WAL archive failures
	MaxTableBloatPct     float64 // Highest table bloat percentage
}

// FetchDBObservationMetrics retrieves critical DBA health metrics
func (c *PgRepository) FetchDBObservationMetrics(instanceName string) (*DBObservationMetrics, error) {
	metrics := &DBObservationMetrics{}

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[instanceName]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return nil, fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return nil, fmt.Errorf("connection not found")
		}
	}

	// Query 1: XID Wraparound %
	var xidAge int64
	var xidPct float64
	err := db.QueryRow(`
		/* SQL_OPTIMA */ SELECT   
			COALESCE(MAX(age(datfrozenxid)), 0),
			COALESCE((MAX(age(datfrozenxid))::float / NULLIF(current_setting('autovacuum_freeze_max_age')::float, 0)) * 100, 0)
		FROM pg_database 
		WHERE datistemplate = false
	`).Scan(&xidAge, &xidPct)
	if err != nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: XID query failed for %s: %v", instanceName, err)
	} else {
		metrics.XIDAge = xidAge
		metrics.XIDWraparoundPct = xidPct
	}

	// Query 2: Index Hit Rate
	err = db.QueryRow(`
		/* SQL_OPTIMA */ SELECT   CASE WHEN (SUM(idx_tup_fetch) + SUM(seq_tup_read)) > 0 
			THEN (SUM(idx_tup_fetch)::float / NULLIF((SUM(idx_tup_fetch) + SUM(seq_tup_read)), 0)) * 100 
			ELSE 0 END
		FROM pg_stat_user_tables
	`).Scan(&metrics.IndexHitPct)
	if err != nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: Index Hit Rate query failed for %s: %v", instanceName, err)
	}

	// Query 3: Idle in Transaction Count
	err = db.QueryRow(`
		/* SQL_OPTIMA */ SELECT   COUNT(*) FROM pg_stat_activity
		WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')
	`).Scan(&metrics.IdleInTransactionCnt)
	if err != nil {
		log.Printf("[POSTGRES] FetchDBObservationMetrics: Idle in Transaction query failed for %s: %v", instanceName, err)
	}

	// Get WAL failed count from ArchiverStats
	archiverStats, err := c.FetchArchiverStats(instanceName)
	if err == nil && archiverStats != nil {
		metrics.WALFails = archiverStats.FailedCount
	}

	// Get max table bloat from table stats
	tableStats, err := c.GetTableStats(instanceName)
	if err == nil && len(tableStats) > 0 {
		for _, t := range tableStats {
			if t.BloatPct > metrics.MaxTableBloatPct {
				metrics.MaxTableBloatPct = t.BloatPct
			}
		}
	}

	return metrics, nil
}
