// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL table and index usage collectors for Storage & Index Health (Timescale monitor.* hypertables).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/domain/storageindex"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// PgIndexUsageRow holds cumulative counters from pg_stat_user_indexes for delta ingestion.
type PgIndexUsageRow struct {
	DBName           string
	SchemaName       string
	TableName        string
	IndexName        string
	IdxScanTotal     int64
	IdxTupReadTotal  int64
	IdxTupFetchTotal int64
	IndexSizeMB      float64
}

// PgTableUsageRow holds pg_stat_user_tables counters plus size snapshots for usage and growth hypertables.
type PgTableUsageRow struct {
	DBName            string
	SchemaName        string
	TableName         string
	SeqScanTotal      int64
	IdxScanTotal      int64
	RowsModifiedTotal int64
	RowCount          int64
	TableSizeMB       float64
	IndexSizeMB       float64
}

// CollectPostgresIndexUsage reads pg_stat_user_indexes (cumulative counters).
func CollectPostgresIndexUsage(ctx context.Context, db *sql.DB) ([]PgIndexUsageRow, error) {
	q := `
		SELECT
			current_database() AS db_name,
			schemaname,
			relname AS table_name,
			indexrelname AS index_name,
			COALESCE(idx_scan, 0) AS idx_scan,
			COALESCE(idx_tup_read, 0) AS idx_tup_read,
			COALESCE(idx_tup_fetch, 0) AS idx_tup_fetch,
			COALESCE(pg_relation_size(indexrelid) / 1024.0 / 1024.0, 0) AS index_size_mb
		FROM pg_stat_user_indexes
	`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgIndexUsageRow
	for rows.Next() {
		var r PgIndexUsageRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.IndexName, &r.IdxScanTotal, &r.IdxTupReadTotal, &r.IdxTupFetchTotal, &r.IndexSizeMB); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CollectPostgresTableUsageAndSize reads pg_stat_user_tables with size estimates.
func CollectPostgresTableUsageAndSize(ctx context.Context, db *sql.DB) ([]PgTableUsageRow, error) {
	q := `
		SELECT
			current_database() AS db_name,
			schemaname,
			relname AS table_name,
			COALESCE(seq_scan, 0) AS seq_scan,
			COALESCE(idx_scan, 0) AS idx_scan,
			COALESCE(n_tup_ins + n_tup_upd + n_tup_del, 0) AS rows_modified,
			COALESCE(n_live_tup, 0) AS row_count,
			COALESCE(pg_total_relation_size(relid) / 1024.0 / 1024.0, 0) AS table_size_mb,
			COALESCE(pg_indexes_size(relid) / 1024.0 / 1024.0, 0) AS index_size_mb
		FROM pg_stat_user_tables
	`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PgTableUsageRow
	for rows.Next() {
		var r PgTableUsageRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.SeqScanTotal, &r.IdxScanTotal, &r.RowsModifiedTotal, &r.RowCount, &r.TableSizeMB, &r.IndexSizeMB); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PersistPostgresIndexUsageDeltas maps PG stats into monitor.index_usage_stats deltas (shared schema with SQL Server).
func PersistPostgresIndexUsageDeltas(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []PgIndexUsageRow, capture time.Time) (inserted int, err error) {
	engine := "postgres"
	for _, r := range rows {
		prev, err := tl.GetIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName)
		if err != nil {
			return inserted, err
		}
		if prev == nil {
			dSeeks, ok1 := storageindex.Delta(r.IdxTupFetchTotal, 0)
			dScans, ok2 := storageindex.Delta(r.IdxScanTotal, 0)
			dLookups, ok3 := storageindex.Delta(r.IdxTupReadTotal, 0)
			if ok1 && ok2 && ok3 {
				stat := models.IndexUsageStat{
					Time:        capture.UTC(),
					Engine:      engine,
					ServerID:    serverID,
					DBName:      r.DBName,
					SchemaName:  r.SchemaName,
					TableName:   r.TableName,
					IndexName:   r.IndexName,
					Seeks:       dSeeks,
					Scans:       dScans,
					Lookups:     dLookups,
					Updates:     0,
					IndexSizeMB: r.IndexSizeMB,
					IsUnique:    false,
					IsPK:        false,
					FillFactor:  0,
				}
				if err := tl.InsertIndexUsageStat(ctx, stat); err != nil {
					log.Printf("[Collector] postgres index_usage_stats bootstrap insert failed: %v", err)
				} else {
					inserted++
				}
			}
			if err := tl.UpsertIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName, r.IdxTupFetchTotal, r.IdxScanTotal, r.IdxTupReadTotal, 0); err != nil {
				return inserted, err
			}
			continue
		}

		dSeeks, ok1 := storageindex.Delta(r.IdxTupFetchTotal, prev.SeeksTotal)
		dScans, ok2 := storageindex.Delta(r.IdxScanTotal, prev.ScansTotal)
		dLookups, ok3 := storageindex.Delta(r.IdxTupReadTotal, prev.LookupsTotal)
		if !(ok1 && ok2 && ok3) {
			_ = tl.UpsertIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName, r.IdxTupFetchTotal, r.IdxScanTotal, r.IdxTupReadTotal, 0)
			continue
		}

		stat := models.IndexUsageStat{
			Time:        capture.UTC(),
			Engine:      engine,
			ServerID:    serverID,
			DBName:      r.DBName,
			SchemaName:  r.SchemaName,
			TableName:   r.TableName,
			IndexName:   r.IndexName,
			Seeks:       dSeeks,
			Scans:       dScans,
			Lookups:     dLookups,
			Updates:     0,
			IndexSizeMB: r.IndexSizeMB,
			IsUnique:    false,
			IsPK:        false,
			FillFactor:  0,
		}
		if err := tl.InsertIndexUsageStat(ctx, stat); err != nil {
			continue
		}
		inserted++
		_ = tl.UpsertIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName, r.IdxTupFetchTotal, r.IdxScanTotal, r.IdxTupReadTotal, 0)
	}
	return inserted, nil
}

// PersistPostgresTableUsageDeltas writes monitor.table_usage_stats deltas from pg_stat_user_tables.
func PersistPostgresTableUsageDeltas(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []PgTableUsageRow, capture time.Time) (inserted int, err error) {
	engine := "postgres"
	for _, r := range rows {
		prev, err := tl.GetTableUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName)
		if err != nil {
			return inserted, err
		}
		if prev == nil {
			if err := tl.UpsertTableUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.SeqScanTotal, r.IdxScanTotal, 0, r.RowsModifiedTotal); err != nil {
				return inserted, err
			}
			continue
		}

		dSeq, ok1 := storageindex.Delta(r.SeqScanTotal, prev.SeqScansTotal)
		dIdx, ok2 := storageindex.Delta(r.IdxScanTotal, prev.IdxScansTotal)
		dMod, ok3 := storageindex.Delta(r.RowsModifiedTotal, prev.RowsModifiedTotal)
		if !(ok1 && ok2 && ok3) {
			_ = tl.UpsertTableUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.SeqScanTotal, r.IdxScanTotal, 0, r.RowsModifiedTotal)
			continue
		}

		stat := models.TableUsageStat{
			Time:         capture.UTC(),
			Engine:       engine,
			ServerID:     serverID,
			DBName:       r.DBName,
			SchemaName:   r.SchemaName,
			TableName:    r.TableName,
			SeqScans:     dSeq,
			IdxScans:     dIdx,
			RowsRead:     0,
			RowsModified: dMod,
			TableSizeMB:  r.TableSizeMB,
			IndexSizeMB:  r.IndexSizeMB,
			RowCount:     r.RowCount,
		}
		if err := tl.InsertTableUsageStat(ctx, stat); err != nil {
			log.Printf("[Collector] pg table_usage_stats insert failed: %v", err)
			continue
		}
		inserted++
		_ = tl.UpsertTableUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.SeqScanTotal, r.IdxScanTotal, 0, r.RowsModifiedTotal)
	}
	return inserted, nil
}

// PersistPostgresTableSizeHistory writes monitor.table_size_history snapshots for growth charts.
func PersistPostgresTableSizeHistory(ctx context.Context, tl *hot.TimescaleLogger, serverID string, rows []PgTableUsageRow, capture time.Time) (inserted int, err error) {
	engine := "postgres"
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
			continue
		}
		inserted++
	}
	return inserted, nil
}
