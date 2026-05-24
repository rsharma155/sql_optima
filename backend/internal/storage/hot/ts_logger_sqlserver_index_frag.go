// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB read/write for sqlserver_index_fragmentation and
// sqlserver_missing_indexes tables.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IndexFragmentationWriteRow is the payload written to sqlserver_index_fragmentation.
type IndexFragmentationWriteRow struct {
	DatabaseName         string
	SchemaName           string
	TableName            string
	IndexName            string
	IndexID              int
	IndexTypeDesc        string
	AvgFragmentationPct  float64
	PageCount            int64
	AvgPageSpaceUsedPct  float64
	RecordCount          int64
	FragmentCount        int64
	AvgFragmentSizePages float64
}

// LogSqlServerIndexFragmentation batch-inserts index fragmentation rows.
func (tl *TimescaleLogger) LogSqlServerIndexFragmentation(ctx context.Context, serverID uuid.UUID, capturedAt time.Time, rows []IndexFragmentationWriteRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	const q = `
		INSERT INTO sqlserver_index_fragmentation (
			capture_timestamp, server_id,
			database_name, schema_name, table_name, index_name, index_id,
			index_type_desc, avg_fragmentation_pct, page_count,
			avg_page_space_used_pct, record_count, fragment_count,
			avg_fragment_size_pages
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
	for _, r := range rows {
		batch.Queue(q, capturedAt, serverID,
			r.DatabaseName, r.SchemaName, r.TableName, r.IndexName, r.IndexID,
			r.IndexTypeDesc, r.AvgFragmentationPct, r.PageCount,
			r.AvgPageSpaceUsedPct, r.RecordCount, r.FragmentCount,
			r.AvgFragmentSizePages,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("sqlserver_index_fragmentation insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

// GetIndexFragmentationForServer returns the latest fragmentation snapshot for a server/database.
func (tl *TimescaleLogger) GetIndexFragmentationForServer(ctx context.Context, serverID uuid.UUID, databaseName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	q := `
		SELECT capture_timestamp, database_name, schema_name, table_name, index_name,
		       index_id, index_type_desc, avg_fragmentation_pct, page_count,
		       avg_page_space_used_pct, record_count
		FROM sqlserver_index_fragmentation
		WHERE server_id = $1
		  AND database_name = $2
		  AND capture_timestamp = (
		      SELECT MAX(capture_timestamp) FROM sqlserver_index_fragmentation
		      WHERE server_id = $1 AND database_name = $2
		  )
		ORDER BY avg_fragmentation_pct DESC
		LIMIT $3`

	rows, err := tl.pool.Query(ctx, q, serverID, databaseName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var dbName, schemaName, tableName, indexName, indexTypeDesc string
		var indexID int
		var fragPct, pageSpacePct float64
		var pageCount, recordCount int64
		if err := rows.Scan(&ts, &dbName, &schemaName, &tableName, &indexName,
			&indexID, &indexTypeDesc, &fragPct, &pageCount, &pageSpacePct, &recordCount,
		); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":       ts,
			"database_name":           dbName,
			"schema_name":             schemaName,
			"table_name":              tableName,
			"index_name":              indexName,
			"index_id":                indexID,
			"index_type_desc":         indexTypeDesc,
			"avg_fragmentation_pct":   fragPct,
			"page_count":              pageCount,
			"avg_page_space_used_pct": pageSpacePct,
			"record_count":            recordCount,
		})
	}
	return out, rows.Err()
}

// MissingIndexWriteRow is the payload written to sqlserver_missing_indexes.
type MissingIndexWriteRow struct {
	DatabaseName      string
	SchemaName        string
	TableName         string
	EqualityColumns   string
	InequalityColumns string
	IncludedColumns   string
	UserSeeks         int64
	UserScans         int64
	AvgTotalUserCost  float64
	AvgUserImpact     float64
	ImprovementScore  float64
	LastUserSeek      *time.Time
	LastUserScan      *time.Time
}

// LogSqlServerMissingIndexes batch-inserts missing index rows.
func (tl *TimescaleLogger) LogSqlServerMissingIndexes(ctx context.Context, serverID uuid.UUID, capturedAt time.Time, rows []MissingIndexWriteRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	const q = `
		INSERT INTO sqlserver_missing_indexes (
			capture_timestamp, server_id,
			database_name, schema_name, table_name,
			equality_columns, inequality_columns, included_columns,
			user_seeks, user_scans,
			avg_total_user_cost, avg_user_impact, improvement_score,
			last_user_seek, last_user_scan
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`
	for _, r := range rows {
		batch.Queue(q, capturedAt, serverID,
			r.DatabaseName, r.SchemaName, r.TableName,
			r.EqualityColumns, r.InequalityColumns, r.IncludedColumns,
			r.UserSeeks, r.UserScans,
			r.AvgTotalUserCost, r.AvgUserImpact, r.ImprovementScore,
			r.LastUserSeek, r.LastUserScan,
		)
	}
	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("sqlserver_missing_indexes insert failed at row %d: %w", i, err)
		}
	}
	return nil
}

// GetMissingIndexesForServer returns the latest missing index snapshot for a server/database.
func (tl *TimescaleLogger) GetMissingIndexesForServer(ctx context.Context, serverID uuid.UUID, databaseName string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `
		SELECT capture_timestamp, database_name, schema_name, table_name,
		       equality_columns, inequality_columns, included_columns,
		       user_seeks, user_scans, avg_total_user_cost, avg_user_impact,
		       improvement_score
		FROM sqlserver_missing_indexes
		WHERE server_id = $1
		  AND database_name = $2
		  AND capture_timestamp = (
		      SELECT MAX(capture_timestamp) FROM sqlserver_missing_indexes
		      WHERE server_id = $1 AND database_name = $2
		  )
		ORDER BY improvement_score DESC
		LIMIT $3`

	rows, err := tl.pool.Query(ctx, q, serverID, databaseName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]interface{}
	for rows.Next() {
		var ts time.Time
		var dbName, schemaName, tableName string
		var eqCols, ineqCols, inclCols string
		var userSeeks, userScans int64
		var avgCost, avgImpact, score float64
		if err := rows.Scan(&ts, &dbName, &schemaName, &tableName,
			&eqCols, &ineqCols, &inclCols,
			&userSeeks, &userScans, &avgCost, &avgImpact, &score,
		); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"capture_timestamp":    ts,
			"database_name":        dbName,
			"schema_name":          schemaName,
			"table_name":           tableName,
			"equality_columns":     eqCols,
			"inequality_columns":   ineqCols,
			"included_columns":     inclCols,
			"user_seeks":           userSeeks,
			"user_scans":           userScans,
			"avg_total_user_cost":  avgCost,
			"avg_user_impact":      avgImpact,
			"improvement_score":    score,
		})
	}
	return out, rows.Err()
}
