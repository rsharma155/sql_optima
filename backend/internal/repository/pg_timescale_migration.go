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
	"context"
	"database/sql"
	"time"
)

// PGTimescaleLockInternal is the internal struct for lock data.
type PGTimescaleLockInternal struct {
	CollectedAt    time.Time
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
	RelationOID    uint32
	RelationName   string
	TransactionID  string
}

// FetchDetailedLocks returns current locks with detailed metadata for historical logging.
func (c *PgRepository) FetchDetailedLocks(ctx context.Context, instanceName string, db *sql.DB) ([]PGTimescaleLockInternal, error) {
	now := time.Now().UTC()
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
			COALESCE(LEFT(a.query, 500), '') as query_text,
			(SELECT pid FROM pg_locks bl WHERE bl.locktype = l.locktype AND bl.database IS NOT DISTINCT FROM l.database AND bl.relation IS NOT DISTINCT FROM l.relation AND bl.page IS NOT DISTINCT FROM l.page AND bl.tuple IS NOT DISTINCT FROM l.tuple AND bl.virtualxid IS NOT DISTINCT FROM l.virtualxid AND bl.transactionid IS NOT DISTINCT FROM l.transactionid AND bl.classid IS NOT DISTINCT FROM l.classid AND bl.objid IS NOT DISTINCT FROM l.objid AND bl.objsubid IS NOT DISTINCT FROM l.objsubid AND bl.granted = true AND bl.pid <> l.pid LIMIT 1) as blocked_by,
			COALESCE(CASE WHEN l.granted = false THEN EXTRACT(EPOCH FROM (now() - a.state_change)) * 1000 ELSE 0 END, 0) as wait_duration_ms,
			COALESCE(l.relation, 0)::oid::integer as relation_oid,
			COALESCE(rel.relname, 'virtual') as relation_name,
			COALESCE(l.transactionid::text, '') as transaction_id
		FROM pg_locks l
		LEFT JOIN pg_stat_activity a ON l.pid = a.pid
		LEFT JOIN pg_class rel ON l.relation = rel.oid
		WHERE l.pid <> pg_backend_pid()
		  AND l.locktype NOT IN ('virtualxid', 'transactionid')
	`
	tctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(tctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locks []PGTimescaleLockInternal
	for rows.Next() {
		var l PGTimescaleLockInternal
		var blockedBy *int
		l.CollectedAt = now
		err := rows.Scan(
			&l.DatabaseName, &l.PID, &l.WaitEventType, &l.WaitEvent,
			&l.LockType, &l.Mode, &l.Granted, &l.QueryText, &blockedBy, &l.WaitDurationMs,
			&l.RelationOID, &l.RelationName, &l.TransactionID,
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
	DbName            string
	UserName          string
	Query             string
	QueryType         string
	Calls             int64
	TotalTime         float64
	MeanTime          float64
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
	TotalPlanTime     float64
	MeanPlanTime      float64
	Plans             int64
}

// FetchStatStatements returns a raw snapshot of pg_stat_statements.
func (c *PgRepository) FetchStatStatements(ctx context.Context, instanceName string, db *sql.DB) ([]PGStatStatementsInternal, error) {
	version := c.GetPgVersion(ctx, instanceName)

	var query string
	if version >= 130000 {
		query = `
			/* SQL_OPTIMA_PHASE8 */
			SELECT 
				queryid, 
				COALESCE(d.datname, 'unknown') as datname,
				COALESCE(u.rolname, 'unknown') as username,
				LEFT(query, 500),
				'O' as query_type,
				calls, 
				total_exec_time,
				(total_exec_time / NULLIF(calls, 0)) as mean_time,
				rows, 
				shared_blks_hit, 
				shared_blks_read, 
				shared_blks_dirtied, 
				shared_blks_written, 
				temp_blks_read, 
				temp_blks_written, 
				blk_read_time, 
				blk_write_time,
				wal_bytes,
				COALESCE(total_plan_time, 0),
				(COALESCE(total_plan_time, 0) / NULLIF(plans, 0)) as mean_plan_time,
				COALESCE(plans, 0)
			FROM pg_stat_statements s
			LEFT JOIN pg_database d ON s.dbid = d.oid
			LEFT JOIN pg_roles u ON s.userid = u.oid
			ORDER BY total_exec_time DESC
			LIMIT 500
		`
	} else {
		query = `
			/* SQL_OPTIMA_PHASE8_LEGACY */
			SELECT 
				queryid, 
				COALESCE(d.datname, 'unknown') as datname,
				COALESCE(u.rolname, 'unknown') as username,
				LEFT(query, 500),
				'O' as query_type,
				calls, 
				total_time,
				(total_time / NULLIF(calls, 0)) as mean_time,
				rows, 
				shared_blks_hit, 
				shared_blks_read, 
				shared_blks_dirtied, 
				shared_blks_written, 
				temp_blks_read, 
				temp_blks_written, 
				blk_read_time, 
				blk_write_time,
				0 as wal_bytes,
				0 as total_plan_time,
				0 as mean_plan_time,
				0 as plans
			FROM pg_stat_statements s
			LEFT JOIN pg_database d ON s.dbid = d.oid
			LEFT JOIN pg_roles u ON s.userid = u.oid
			ORDER BY total_time DESC
			LIMIT 500
		`
	}

	tctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(tctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PGStatStatementsInternal
	for rows.Next() {
		var s PGStatStatementsInternal
		err := rows.Scan(
			&s.QueryID, &s.DbName, &s.UserName, &s.Query, &s.QueryType, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows,
			&s.SharedBlksHit, &s.SharedBlksRead, &s.SharedBlksDirtied, &s.SharedBlksWritten,
			&s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime, &s.WalBytes,
			&s.TotalPlanTime, &s.MeanPlanTime, &s.Plans,
		)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats, nil
}
