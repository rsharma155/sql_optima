// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL repository methods for Storage & Index Health.
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

// PgIndexUsageStatInternal represents index usage statistics for collection.
type PgIndexUsageStatInternal struct {
	CaptureTimestamp time.Time
	DbName           string
	SchemaName       string
	TableName        string
	IndexName        string
	IdxScan          int64
	IdxTupRead       int64
	IdxTupFetch      int64
	IndexSizeMB      float64
	IsPK             bool
}

// FetchPgIndexUsageStats returns index usage statistics for all user indexes.
func (c *PgRepository) FetchPgIndexUsageStats(ctx context.Context, instanceName string, db *sql.DB) ([]PgIndexUsageStatInternal, error) {
	now := time.Now().UTC()
	query := `
		/* SQL_OPTIMA_SIH */
		SELECT 
			current_database() as db_name,
			s.schemaname,
			s.relname as table_name,
			s.indexrelname as index_name,
			COALESCE(s.idx_scan, 0) as idx_scan,
			COALESCE(s.idx_tup_read, 0) as idx_tup_read,
			COALESCE(s.idx_tup_fetch, 0) as idx_tup_fetch,
			pg_relation_size(s.indexrelid) / 1024.0 / 1024.0 as index_size_mb,
			EXISTS (
				SELECT 1 FROM pg_index i 
				JOIN pg_class c ON i.indexrelid = c.oid 
				WHERE i.indexrelid = s.indexrelid AND i.indisprimary
			) as is_pk
		FROM pg_stat_user_indexes s
		WHERE s.schemaname NOT IN ('pg_catalog', 'information_schema')
	`
	tctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(tctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []PgIndexUsageStatInternal
	for rows.Next() {
		var s PgIndexUsageStatInternal
		s.CaptureTimestamp = now
		err := rows.Scan(
			&s.DbName, &s.SchemaName, &s.TableName, &s.IndexName,
			&s.IdxScan, &s.IdxTupRead, &s.IdxTupFetch, &s.IndexSizeMB, &s.IsPK,
		)
		if err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats, nil
}

// FetchPgTableUsageStats returns table usage statistics for all user tables.
func (c *PgRepository) FetchPgTableUsageStats(ctx context.Context, instanceName string, db *sql.DB) ([]PgTableMaintenanceStat, error) {
	return c.GetTableMaintenanceStatsForDB(ctx, db, 1000)
}
