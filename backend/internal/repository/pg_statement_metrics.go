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
)

// PgQueryStat represents query performance statistics from pg_stat_statements.
type PgQueryStat struct {
	QueryID           int64   `json:"query_id"`
	Query             string  `json:"query"`
	UserName          string  `json:"user,omitempty"`
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

// GetQueryStats returns top queries by total execution time from pg_stat_statements.
// Stats are cumulative since the last pg_stat_statements_reset() (not a time window).
// Requires pg_stat_statements extension to be installed.
func (c *PgRepository) GetQueryStats(instanceName string) ([]PgQueryStat, error) {
	return c.GetQueryStatsWithLimit(instanceName, 50)
}

// GetQueryStatsWithLimit is like GetQueryStats but allows a higher LIMIT for Timescale snapshots (delta windows).
func (c *PgRepository) GetQueryStatsWithLimit(instanceName string, limit int) ([]PgQueryStat, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 5000 {
		limit = 5000
	}

	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Check if pg_stat_statements is available
	var exists bool
	err := db.QueryRow("SELECT /* SQL_OPTIMA */   EXISTS (SELECT /* SQL_OPTIMA */   1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	query := `SELECT /* SQL_OPTIMA */   
			s.queryid,
			LEFT(s.query, 400) as query,
			COALESCE(r.rolname, '') as user_name,
			s.calls,
			s.total_exec_time,
			s.mean_exec_time,
			s.rows,
			s.temp_blks_read,
			s.temp_blks_written,
			s.blk_read_time,
			s.blk_write_time,
			s.shared_blks_read,
			s.shared_blks_hit,
			COALESCE(s.wal_bytes,0)
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE ` + buildPgStatStatementsFilters() + fmt.Sprintf(`
		ORDER BY s.total_exec_time DESC
		LIMIT %d
	`, limit)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgQueryStat
	for rows.Next() {
		var s PgQueryStat
		err := rows.Scan(&s.QueryID, &s.Query, &s.UserName, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows, &s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime, &s.SharedBlksRead, &s.SharedBlksHit, &s.WalBytes)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// GetQueryStatsForSnapshot returns all matching pg_stat_statements rows (no LIMIT) for Timescale delta snapshots.
func (c *PgRepository) GetQueryStatsForSnapshot(instanceName string) ([]PgQueryStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	var exists bool
	err := db.QueryRow("SELECT /* SQL_OPTIMA */   EXISTS (SELECT /* SQL_OPTIMA */   1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	query := `/* SQL_OPTIMA */ SELECT   
			s.queryid,
			LEFT(s.query, 400) as query,
			COALESCE(r.rolname, '') as user_name,
			s.calls,
			s.total_exec_time,
			s.mean_exec_time,
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
			COALESCE(s.mean_plan_time,0),
			COALESCE(s.plans,0)
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE ` + buildPgStatStatementsFilters() + `
		ORDER BY s.queryid
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgQueryStat
	for rows.Next() {
		var s PgQueryStat
		err := rows.Scan(&s.QueryID, &s.Query, &s.UserName, &s.Calls, &s.TotalTime, &s.MeanTime, &s.Rows,
			&s.TempBlksRead, &s.TempBlksWritten, &s.BlkReadTime, &s.BlkWriteTime,
			&s.SharedBlksRead, &s.SharedBlksHit, &s.SharedBlksDirtied, &s.SharedBlksWritten,
			&s.WalBytes, &s.WalRecords, &s.WalFpi,
			&s.TotalPlanTime, &s.MeanPlanTime, &s.Plans)
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

// FetchNormalizedQueryStats retrieves query statistics from pg_stat_statements.
// Returns query ID, text, and execution metrics. Requires pg_stat_statements extension.
func (c *PgRepository) FetchNormalizedQueryStats(instanceName string) ([]NormalizedQueryStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	// Check if pg_stat_statements is available
	var exists bool
	err := db.QueryRow("/* SQL_OPTIMA */ SELECT   EXISTS (SELECT   1 FROM pg_extension WHERE extname = 'pg_stat_statements')").Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("pg_stat_statements extension not available")
	}

	query := `/* SQL_OPTIMA */ SELECT   
			s.queryid,
			s.query,
			s.calls,
			s.total_exec_time,
			s.mean_exec_time,
			s.rows
		FROM pg_stat_statements s
		LEFT JOIN pg_roles r ON r.oid = s.userid
		WHERE ` + buildPgStatStatementsFilters() + `
		ORDER BY s.total_exec_time DESC
		LIMIT 100
	`

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
	db, ok := c.conns[instanceName]
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
