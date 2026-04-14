// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Table usage statistics collector for access pattern analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type SqlServerTableUsageRow struct {
	DBName      string
	SchemaName  string
	TableName   string
	RowCount    int64
	TableSizeMB float64
	IndexSizeMB float64
}

// CollectSQLServerTableSizeSnapshot returns table size + row_count. SQL Server doesn't expose seq_scan/idx_scan,
// so those are stored as zeros (dashboard uses index_usage_stats for seek/scan patterns).
func CollectSQLServerTableSizeSnapshot(ctx context.Context, dbq Queryer) ([]SqlServerTableUsageRow, error) {
	q := `
		SELECT
			DB_NAME() AS db_name,
			s.name AS schema_name,
			t.name AS table_name,
			SUM(p.rows) AS row_count,
			CAST(SUM(a.total_pages) * 8.0 / 1024.0 AS float) AS table_size_mb,
			CAST(SUM(a.used_pages) * 8.0 / 1024.0 AS float) AS index_size_mb
		FROM sys.tables t
		JOIN sys.schemas s ON t.schema_id = s.schema_id
		JOIN sys.partitions p ON t.object_id = p.object_id
		JOIN sys.allocation_units a ON p.partition_id = a.container_id
		WHERE t.is_ms_shipped = 0
		GROUP BY s.name, t.name
	`

	rows, err := dbq.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SqlServerTableUsageRow
	for rows.Next() {
		var r SqlServerTableUsageRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.RowCount, &r.TableSizeMB, &r.IndexSizeMB); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func PersistSQLServerTableUsageDeltas(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []SqlServerTableUsageRow, capture time.Time) (inserted int, err error) {
	engine := "sqlserver"
	for _, r := range rows {
		// SQL Server: no cumulative scan counters. We still upsert 0 totals to keep uniform state table.
		prev, err := tl.GetTableUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName)
		if err != nil {
			return inserted, fmt.Errorf("get table usage state: %w", err)
		}
		if prev == nil {
			// seed state to zeros
			if err := tl.UpsertTableUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, 0, 0, 0, 0); err != nil {
				return inserted, fmt.Errorf("seed table usage state: %w", err)
			}
		}

		// Store a “delta” row with zeros for scan counters; sizes + row_count as snapshot values.
		stat := models.TableUsageStat{
			Time:         capture.UTC(),
			Engine:       engine,
			ServerID:     serverID,
			DBName:       r.DBName,
			SchemaName:   r.SchemaName,
			TableName:    r.TableName,
			SeqScans:     0,
			IdxScans:     0,
			RowsRead:     0,
			RowsModified: 0,
			TableSizeMB:  r.TableSizeMB,
			IndexSizeMB:  r.IndexSizeMB,
			RowCount:     r.RowCount,
		}
		if err := tl.InsertTableUsageStat(ctx, stat); err != nil {
			log.Printf("[Collector] table_usage_stats insert failed: %v", err)
			continue
		}
		inserted++
	}
	return inserted, nil
}

func PersistSQLServerTableSizeHistory(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []SqlServerTableUsageRow, capture time.Time) (inserted int, err error) {
	engine := "sqlserver"
	for _, r := range rows {
		h := models.TableSizeHistory{
			Time:        capture.UTC(),
			Engine:      engine,
			ServerID:    serverID,
			DBName:      r.DBName,
			SchemaName:  r.SchemaName,
			TableName:   r.TableName,
			TableSizeMB: r.TableSizeMB,
			IndexSizeMB: r.IndexSizeMB,
			RowCount:    r.RowCount,
		}
		if err := tl.InsertTableSizeHistory(ctx, h); err != nil {
			log.Printf("[Collector] table_size_history insert failed: %v", err)
			continue
		}
		inserted++
	}
	return inserted, nil
}
