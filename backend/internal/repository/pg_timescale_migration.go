// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository methods for PostgreSQL TimescaleDB migration (Phase 8).
// Prefix: pg_ (as requested)
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package repository

import (
	"fmt"
	"strings"
)

// PGTimescaleLockInternal is the internal struct for lock data.
type PGTimescaleLockInternal struct {
	DatabaseName   string
	PID            int
	WaitEventType  string
	WaitEvent      string
	LockType       string
	Mode           string
	Granted        bool
	QueryText      string
	BlockedBy      int
	WaitDurationMs float64
}

// FetchDetailedLocks returns current locks with detailed metadata for historical logging.
func (c *PgRepository) FetchDetailedLocks(instanceName string) ([]PGTimescaleLockInternal, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA_PHASE8 */
		SELECT 
			COALESCE(a.datname, 'unknown') as database_name,
			l.pid,
			COALESCE(a.wait_event_type, 'Lock') as wait_event_type,
			COALESCE(a.wait_event, l.locktype) as wait_event,
			l.locktype,
			l.mode,
			l.granted,
			LEFT(a.query, 500) as query_text,
			(SELECT pid FROM pg_locks bl WHERE bl.locktype = l.locktype AND bl.database IS NOT DISTINCT FROM l.database AND bl.relation IS NOT DISTINCT FROM l.relation AND bl.page IS NOT DISTINCT FROM l.page AND bl.tuple IS NOT DISTINCT FROM l.tuple AND bl.virtualxid IS NOT DISTINCT FROM l.virtualxid AND bl.transactionid IS NOT DISTINCT FROM l.transactionid AND bl.classid IS NOT DISTINCT FROM l.classid AND bl.objid IS NOT DISTINCT FROM l.objid AND bl.objsubid IS NOT DISTINCT FROM l.objsubid AND bl.granted = true AND bl.pid <> l.pid LIMIT 1) as blocked_by,
			CASE WHEN l.granted = false THEN EXTRACT(EPOCH FROM (now() - a.state_change)) * 1000 ELSE 0 END as wait_duration_ms
		FROM pg_locks l
		LEFT JOIN pg_stat_activity a ON l.pid = a.pid
		WHERE l.pid <> pg_backend_pid()
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locks []PGTimescaleLockInternal
	for rows.Next() {
		var l PGTimescaleLockInternal
		var blockedBy *int
		err := rows.Scan(
			&l.DatabaseName, &l.PID, &l.WaitEventType, &l.WaitEvent,
			&l.LockType, &l.Mode, &l.Granted, &l.QueryText, &blockedBy, &l.WaitDurationMs,
		)
		if err != nil {
			continue
		}
		if blockedBy != nil {
			l.BlockedBy = *blockedBy
		}
		locks = append(locks, l)
	}
	return locks, nil
}

// PGStatStatementsInternal is the internal struct for pg_stat_statements.
type PGStatStatementsInternal struct {
	QueryID           int64
	DatabaseName      string
	UserName          string
	Calls             int64
	TotalTime         float64
	Rows              int64
	SharedBlksHit     int64
	SharedBlksRead    int64
	SharedBlksDirtied int64
	SharedBlksWritten int64
	TempBlksRead      int64
	TempBlksWritten   int64
	BlkReadTime       float64
	BlkWriteTime      float64
	WalBytes          float64
}

// FetchStatStatements returns a raw snapshot of pg_stat_statements.
func (c *PgRepository) FetchStatStatements(instanceName string) ([]PGStatStatementsInternal, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	version := c.GetPgVersion(instanceName)

	var query string
	if version >= 130000 {
		query = `
			/* SQL_OPTIMA_PHASE8 */
			SELECT 
				queryid, 
				COALESCE(d.datname, 'unknown') as datname,
				COALESCE(u.rolname, 'unknown') as username,
				calls, 
				total_exec_time + total_plan_time as total_time,
				rows, 
				shared_blks_hit, 
				shared_blks_read, 
				shared_blks_dirtied, 
				shared_blks_written, 
				temp_blks_read, 
				temp_blks_written, 
				blk_read_time, 
				blk_write_time,
				wal_bytes
			FROM pg_stat_statements s
			LEFT JOIN pg_database d ON s.dbid = d.oid
			LEFT JOIN pg_roles u ON s.userid = u.oid
			LIMIT 500
		`
	} else {
		query = `
			/* SQL_OPTIMA_PHASE8_LEGACY */
			SELECT 
				queryid, 
				COALESCE(d.datname, 'unknown') as datname,
				COALESCE(u.rolname, 'unknown') as username,
				calls, 
				total_time,
				rows, 
				shared_blks_hit, 
				shared_blks_read, 
				shared_blks_dirtied, 
				shared_blks_written, 
				temp_blks_read, 
				temp_blks_written, 
				blk_read_time, 
				blk_write_time,
				0 as wal_bytes
			FROM pg_stat_statements s
			LEFT JOIN pg_database d ON s.dbid = d.oid
			LEFT JOIN pg_roles u ON s.userid = u.oid
			LIMIT 500
		`
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PGStatStatementsInternal
	for rows.Next() {
		var s PGStatStatementsInternal
		err := rows.Scan(
			&s.QueryID, &s.DatabaseName, &s.UserName, &s.Calls, &s.TotalTime, &s.Rows,
			&s.SharedBlksHit, &s.SharedBlksRead, &s.SharedBlksDirtied, &s.SharedBlksWritten,
			&s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime, &s.WalBytes,
		)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats, nil
}
