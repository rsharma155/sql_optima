// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL storage metrics for tables and indexes.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"strings"
)

// PgTableStat represents table storage and vacuum statistics.
type PgTableStat struct {
	Schema      string  `json:"schema"`
	Table       string  `json:"table"`
	TotalSize   string  `json:"total_size"`
	DeadTuples  int64   `json:"dead_tuples"`
	BloatPct    float64 `json:"bloat_pct"`
	SeqScans    int64   `json:"seq_scans"`
	IdxScans    int64   `json:"idx_scans"`
	LastVacuum  *string `json:"last_vacuum"`
	LastAnalyze *string `json:"last_analyze"`
}

// GetTableStats returns table statistics including size, bloat, and vacuum information.
func (c *PgRepository) GetTableStats(instanceName string) ([]PgTableStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT   
			schemaname,
			relname,
			pg_size_pretty(pg_total_relation_size(quote_ident(schemaname)||'.'||quote_ident(relname))) as total_size,
			n_dead_tup,
			CASE WHEN n_live_tup + n_dead_tup > 0 THEN (n_dead_tup::float / (n_live_tup + n_dead_tup)) * 100 ELSE 0 END as bloat_pct,
			seq_scan,
			idx_scan,
			last_vacuum,
			last_analyze
		FROM pg_stat_user_tables
		ORDER BY pg_total_relation_size(quote_ident(schemaname)||'.'||quote_ident(relname)) DESC
		LIMIT 20
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgTableStat
	for rows.Next() {
		var s PgTableStat
		err := rows.Scan(&s.Schema, &s.Table, &s.TotalSize, &s.DeadTuples, &s.BloatPct, &s.SeqScans, &s.IdxScans, &s.LastVacuum, &s.LastAnalyze)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// PgIndexStat represents index usage statistics.
type PgIndexStat struct {
	IndexName string `json:"index_name"`
	TableName string `json:"table_name"`
	Size      string `json:"size"`
	Scans     int64  `json:"scans"`
	Reason    string `json:"reason"`
}

// GetIndexStats returns potentially unused indexes (idx_scan = 0).
func (c *PgRepository) GetIndexStats(instanceName string) ([]PgIndexStat, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT   
			indexrelname,
			relname,
			pg_size_pretty(pg_relation_size(indexrelid)) as size,
			idx_scan,
			CASE 
				WHEN idx_scan = 0 THEN 'Unused'
				ELSE 'OK'
			END as reason
		FROM pg_stat_user_indexes
		WHERE idx_scan = 0
		ORDER BY pg_relation_size(indexrelid) DESC
		LIMIT 10
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgIndexStat
	for rows.Next() {
		var s PgIndexStat
		err := rows.Scan(&s.IndexName, &s.TableName, &s.Size, &s.Scans, &s.Reason)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}

	return stats, nil
}
