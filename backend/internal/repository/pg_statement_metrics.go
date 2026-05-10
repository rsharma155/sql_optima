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
	"fmt"
	"log"
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
func (c *PgRepository) FetchQuerySnapshot(instanceName string) ([]PgQueryStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	supported := c.pgssSupported[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found for instance: %s", instanceName)
	}

	if !supported {
		return nil, fmt.Errorf("pg_stat_statements not supported or enabled on %s", instanceName)
	}

	version := c.GetPgVersion(instanceName)

	var query string
	if version >= 130000 {
		query = `/* SQL_OPTIMA */
			SELECT
				s.queryid,
				s.userid,
				s.dbid,
				d.datname                               AS db_name,
				COALESCE(r.rolname, s.userid::text)     AS username,
				LEFT(s.query, 500),
				s.calls,
				s.total_exec_time,
				s.rows,
				s.temp_blks_read,
				s.temp_blks_written,
				s.blk_read_time,
				s.blk_write_time,
				s.shared_blks_read,
				s.shared_blks_hit,
				COALESCE(s.shared_blks_dirtied,0),
				COALESCE(s.shared_blks_written,0),
				COALESCE(s.wal_bytes,0),
				COALESCE(s.wal_records,0),
				COALESCE(s.wal_fpi,0),
				COALESCE(s.total_plan_time,0),
				COALESCE(s.plans,0)
			FROM pg_stat_statements s
			JOIN  pg_database d ON d.oid = s.dbid
			LEFT JOIN pg_roles  r ON r.oid = s.userid
			WHERE s.query NOT LIKE '%/* SQL_OPTIMA */%'
			  AND d.datname NOT IN ('template0', 'template1')
		`
	} else {
		query = `/* SQL_OPTIMA */
			SELECT
				s.queryid,
				s.userid,
				s.dbid,
				d.datname                               AS db_name,
				COALESCE(r.rolname, s.userid::text)     AS username,
				LEFT(s.query, 500),
				s.calls,
				s.total_time,
				s.rows,
				s.temp_blks_read,
				s.temp_blks_written,
				s.blk_read_time,
				s.blk_write_time,
				s.shared_blks_read,
				s.shared_blks_hit,
				COALESCE(s.shared_blks_dirtied,0),
				COALESCE(s.shared_blks_written,0),
				0 as wal_bytes,
				0 as wal_records,
				0 as wal_fpi,
				0 as total_plan_time,
				0 as plans
			FROM pg_stat_statements s
			JOIN  pg_database d ON d.oid = s.dbid
			LEFT JOIN pg_roles  r ON r.oid = s.userid
			WHERE s.query NOT LIKE '%/* SQL_OPTIMA */%'
			  AND d.datname NOT IN ('template0', 'template1')
		`
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgQueryStat
	for rows.Next() {
		var s PgQueryStat
		err := rows.Scan(
			&s.QueryID, &s.UserID, &s.DbID, &s.DbName, &s.UserName,
			&s.Query, &s.Calls, &s.TotalTime, &s.Rows,
			&s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime,
			&s.SharedBlksRead, &s.SharedBlksHit, &s.SharedBlksDirtied, &s.SharedBlksWritten,
			&s.WalBytes, &s.WalRecords, &s.WalFpi,
			&s.TotalPlanTime, &s.Plans,
		)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// ComputeDelta calculates the difference between current and previous snapshots.
// Returns the delta row and a boolean indicating if a meaningful change exists.
// If a reset is detected (counters decreased), it returns (current, false).
func ComputeDelta(current, previous PgQueryStat) (PgQueryStat, bool) {
	// Detect pg_stat_statements reset
	if current.Calls < previous.Calls || current.TotalTime < previous.TotalTime {
		return current, false
	}

	delta := current
	delta.Calls = current.Calls - previous.Calls
	delta.TotalTime = current.TotalTime - previous.TotalTime
	delta.Rows = current.Rows - previous.Rows
	delta.TempBlksRead = current.TempBlksRead - previous.TempBlksRead
	delta.TempBlksWritten = current.TempBlksWritten - previous.TempBlksWritten
	delta.BlkReadTime = current.BlkReadTime - previous.BlkReadTime
	delta.BlkWriteTime = current.BlkWriteTime - previous.BlkWriteTime
	delta.SharedBlksRead = current.SharedBlksRead - previous.SharedBlksRead
	delta.SharedBlksHit = current.SharedBlksHit - previous.SharedBlksHit
	delta.SharedBlksDirtied = current.SharedBlksDirtied - previous.SharedBlksDirtied
	delta.SharedBlksWritten = current.SharedBlksWritten - previous.SharedBlksWritten
	delta.WalBytes = current.WalBytes - previous.WalBytes
	delta.WalRecords = current.WalRecords - previous.WalRecords
	delta.WalFpi = current.WalFpi - previous.WalFpi
	delta.TotalPlanTime = current.TotalPlanTime - previous.TotalPlanTime
	delta.Plans = current.Plans - previous.Plans

	// If no calls occurred, no meaningful change
	if delta.Calls <= 0 {
		return delta, false
	}

	// Recompute means
	if delta.Calls > 0 {
		delta.MeanTime = delta.TotalTime / float64(delta.Calls)
	}
	if delta.Plans > 0 {
		delta.MeanPlanTime = delta.TotalPlanTime / float64(delta.Plans)
	}

	return delta, true
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

// FetchNormalizedQueryStats retrieves query statistics from pg_stat_statements.
// Returns query ID, text, and execution metrics. Requires pg_stat_statements extension.
func (c *PgRepository) FetchNormalizedQueryStats(instanceName string) ([]NormalizedQueryStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	if !c.GetPgssSupported(instanceName) {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	version := c.GetPgVersion(instanceName)
	timeCol := "total_exec_time"
	meanCol := "mean_exec_time"
	if version < 130000 {
		timeCol = "total_time"
		meanCol = "total_time / NULLIF(calls, 0)"
	}

	query := fmt.Sprintf(`/* SQL_OPTIMA */ SELECT   
			s.queryid,
			s.query,
			s.calls,
			s.%s,
			%s,
			s.rows
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE %s
		ORDER BY s.%s DESC
		LIMIT 100
	`, timeCol, meanCol, buildPgStatStatementsFilters(), timeCol)

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []NormalizedQueryStat
	for rows.Next() {
		var s NormalizedQueryStat
		if err := rows.Scan(&s.QueryID, &s.QueryText, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows); err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, rows.Err()
}

// ResetQueryStats resets pg_stat_statements statistics.
func (c *PgRepository) ResetQueryStats(instanceName string) error {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return fmt.Errorf("connection not found")
	}

	_, err := db.Exec("/* SQL_OPTIMA */ SELECT   pg_stat_statements_reset()")
	if err != nil {
		return fmt.Errorf("failed to reset query stats: %w", err)
	}

	log.Printf("[POSTGRES] Reset pg_stat_statements on %s", instanceName)
	return nil
}
