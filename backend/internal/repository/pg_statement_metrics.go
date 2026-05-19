// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL query performance statistics via pg_stat_statements.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// PgQueryStat represents query performance statistics from pg_stat_statements.
type PgQueryStat struct {
	QueryID           int64   `json:"query_id"`
	UserID            int64   `json:"user_id"`
	DbID              int64   `json:"db_id"`
	DbName            string  `json:"db_name"`
	UserName          string  `json:"username"`
	Query             string  `json:"query"`
	QueryType         string  `json:"query_type"`
	Calls             int64   `json:"calls"`
	TotalTime         float64 `json:"total_time"`
	MeanTime          float64 `json:"mean_time"`
	Rows              int64   `json:"rows"`
	TempBlksRead      int64   `json:"temp_blks_read"`
	TempBlksWritten   int64   `json:"temp_blks_written"`
	BlkReadTime       float64 `json:"blk_read_time"`
	BlkWriteTime      float64 `json:"blk_write_time"`
	SharedBlksRead    int64   `json:"shared_blks_read,omitempty"`
	SharedBlksHit     int64   `json:"shared_blks_hit,omitempty"`
	SharedBlksDirtied int64   `json:"shared_blks_dirtied,omitempty"`
	SharedBlksWritten int64   `json:"shared_blks_written,omitempty"`
	WalBytes          int64   `json:"wal_bytes,omitempty"`
	WalRecords        int64   `json:"wal_records,omitempty"`
	WalFpi            int64   `json:"wal_fpi,omitempty"`
	TotalPlanTime     float64 `json:"total_plan_time,omitempty"`
	MeanPlanTime      float64 `json:"mean_plan_time,omitempty"`
	Plans             int64   `json:"plans,omitempty"`
}

// FetchQuerySnapshot returns a raw snapshot of pg_stat_statements.
func (c *PgRepository) FetchQuerySnapshot(ctx context.Context, instanceName string, db *sql.DB) ([]PgQueryStat, error) {
	c.mutex.RLock()
	supported := c.pgssSupported[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !supported {
		return nil, fmt.Errorf("pg_stat_statements not supported or enabled on %s", instanceName)
	}

	version := c.GetPgVersion(ctx, instanceName)
	hasBlkTime := c.GetPgssHasBlkTime(instanceName)

	// Choose the exec-time column name: total_exec_time (PG13+) vs total_time (older).
	execTimeCol := "s.total_time"
	if version >= 130000 {
		execTimeCol = "s.total_exec_time"
	}

	// Choose blk I/O time columns: use literal 0 when the column doesn't exist in the
	// extension schema (can happen on any version with an old extension install).
	blkReadCol := "0"
	blkWriteCol := "0"
	if hasBlkTime {
		blkReadCol = "s.blk_read_time"
		blkWriteCol = "s.blk_write_time"
	}

	// PG13+ exposes WAL counters and plan-time columns; older versions don't.
	walCols := "0 as wal_bytes, 0 as wal_records, 0 as wal_fpi, 0 as total_plan_time, 0 as mean_plan_time, 0 as plans"
	if version >= 130000 {
		walCols = "COALESCE(s.wal_bytes,0), COALESCE(s.wal_records,0), COALESCE(s.wal_fpi,0), COALESCE(s.total_plan_time,0), (COALESCE(s.total_plan_time,0)/NULLIF(s.plans,0)), COALESCE(s.plans,0)"
	}

	query := fmt.Sprintf(`/* SQL_OPTIMA */
		SELECT
			s.queryid,
			s.userid,
			s.dbid,
			d.datname                                        AS db_name,
			COALESCE(r.rolname, s.userid::text)              AS username,
			LEFT(s.query, 500),
			'O' as query_type,
			s.calls,
			%s,
			(%s / NULLIF(s.calls, 0))                        AS mean_time,
			s.rows,
			s.temp_blks_read,
			s.temp_blks_written,
			%s                                               AS blk_read_time,
			%s                                               AS blk_write_time,
			s.shared_blks_read,
			s.shared_blks_hit,
			COALESCE(s.shared_blks_dirtied,0),
			COALESCE(s.shared_blks_written,0),
			%s
		FROM pg_stat_statements s
		JOIN  pg_database d ON d.oid = s.dbid
		LEFT JOIN pg_roles  r ON r.oid = s.userid
		WHERE s.query NOT LIKE '%%/* SQL_OPTIMA */%%'
		  AND d.datname NOT IN ('template0', 'template1')
		ORDER BY %s DESC
		LIMIT 200
	`, execTimeCol, execTimeCol, blkReadCol, blkWriteCol, walCols, execTimeCol)


	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgQueryStat
	for rows.Next() {
		var s PgQueryStat
		err := rows.Scan(
			&s.QueryID, &s.UserID, &s.DbID, &s.DbName, &s.UserName,
			&s.Query, &s.QueryType, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows,
			&s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime,
			&s.SharedBlksRead, &s.SharedBlksHit, &s.SharedBlksDirtied, &s.SharedBlksWritten,
			&s.WalBytes, &s.WalRecords, &s.WalFpi,
			&s.TotalPlanTime, &s.MeanPlanTime, &s.Plans,
		)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// NormalizedQueryStat represents normalized query statistics with query ID.
type NormalizedQueryStat struct {
	QueryID   int64
	QueryText string
	Calls     int64
	TotalTime float64
	MeanTime  float64
	Rows      int64
}

// ResetQueryStats resets pg_stat_statements statistics.
func (c *PgRepository) ResetQueryStats(ctx context.Context, instanceName string) error {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return fmt.Errorf("connection not found")
	}

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	_, err := db.ExecContext(ctx, "/* SQL_OPTIMA */ SELECT   pg_stat_statements_reset()")
	if err != nil {
		return fmt.Errorf("failed to reset query stats: %w", err)
	}

	slog.Info("[POSTGRES] Reset pg_stat_statements on", "val", instanceName)
	return nil
}

// ComputeDelta calculates the difference between two PgQueryStat snapshots.
// Returns a delta PgQueryStat and true if there was a meaningful change.
func ComputeDelta(curr, prev PgQueryStat) (PgQueryStat, bool) {
	if curr.Calls < prev.Calls {
		// Reset detected
		return PgQueryStat{}, false
	}

	delta := PgQueryStat{
		QueryID:           curr.QueryID,
		UserID:            curr.UserID,
		DbID:              curr.DbID,
		DbName:            curr.DbName,
		UserName:          curr.UserName,
		Query:             curr.Query,
		QueryType:         curr.QueryType,
		Calls:             curr.Calls - prev.Calls,
		TotalTime:         curr.TotalTime - prev.TotalTime,
		Rows:              curr.Rows - prev.Rows,
		TempBlksRead:      curr.TempBlksRead - prev.TempBlksRead,
		TempBlksWritten:   curr.TempBlksWritten - prev.TempBlksWritten,
		BlkReadTime:       curr.BlkReadTime - prev.BlkReadTime,
		BlkWriteTime:      curr.BlkWriteTime - prev.BlkWriteTime,
		SharedBlksRead:    curr.SharedBlksRead - prev.SharedBlksRead,
		SharedBlksHit:     curr.SharedBlksHit - prev.SharedBlksHit,
		SharedBlksDirtied: curr.SharedBlksDirtied - prev.SharedBlksDirtied,
		SharedBlksWritten: curr.SharedBlksWritten - prev.SharedBlksWritten,
		WalBytes:          curr.WalBytes - prev.WalBytes,
		WalRecords:        curr.WalRecords - prev.WalRecords,
		WalFpi:            curr.WalFpi - prev.WalFpi,
		TotalPlanTime:     curr.TotalPlanTime - prev.TotalPlanTime,
		Plans:             curr.Plans - prev.Plans,
	}

	if delta.Calls > 0 {
		delta.MeanTime = delta.TotalTime / float64(delta.Calls)
		if delta.Plans > 0 {
			delta.MeanPlanTime = delta.TotalPlanTime / float64(delta.Plans)
		}
		return delta, true
	}

	return PgQueryStat{}, false
}
