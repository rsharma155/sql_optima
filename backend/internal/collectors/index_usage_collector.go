// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Index usage statistics collector for optimization recommendations.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/domain/storageindex"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type SqlServerIndexUsageRow struct {
	DBName         string
	SchemaName     string
	TableName      string
	IndexName      string
	SeeksTotal     int64
	ScansTotal     int64
	LookupsTotal   int64
	UpdatesTotal   int64
	IndexSizeMB    float64
	IsUnique       bool
	IsPK           bool
	FillFactor     int
	LastUserSeek   sql.NullTime
	LastUserScan   sql.NullTime
	LastUserLookup sql.NullTime
}

func CollectSQLServerIndexUsage(ctx context.Context, dbq Queryer) ([]SqlServerIndexUsageRow, error) {
	// NOTE: counters are cumulative; deltas are computed against monitor.index_usage_state.
	// Aggregate pages per (object_id, index_id) in a subquery so we do not need a GROUP BY
	// that would make s.last_user_* invalid in the SELECT list (SQL Server rejects that).
	query := `
		SELECT
			DB_NAME() AS db_name,
			OBJECT_SCHEMA_NAME(i.object_id) AS schema_name,
			OBJECT_NAME(i.object_id) AS table_name,
			i.name AS index_name,
			COALESCE(s.user_seeks, 0) AS user_seeks,
			COALESCE(s.user_scans, 0) AS user_scans,
			COALESCE(s.user_lookups, 0) AS user_lookups,
			COALESCE(s.user_updates, 0) AS user_updates,
			s.last_user_seek,
			s.last_user_scan,
			s.last_user_lookup,
			CAST(COALESCE(sz.total_pages, 0) * 8.0 / 1024.0 AS float) AS index_size_mb,
			CAST(i.is_unique AS bit) AS is_unique,
			CASE WHEN i.is_primary_key = 1 THEN CAST(1 AS bit) ELSE CAST(0 AS bit) END AS is_pk,
			COALESCE(NULLIF(i.fill_factor, 0), 0) AS fillfactor
		FROM sys.indexes i
		LEFT JOIN sys.dm_db_index_usage_stats s
		       ON s.database_id = DB_ID() AND s.object_id = i.object_id AND s.index_id = i.index_id
		LEFT JOIN (
			SELECT p.object_id, p.index_id, SUM(a.total_pages) AS total_pages
			FROM sys.partitions p
			INNER JOIN sys.allocation_units a ON a.container_id = p.partition_id
			GROUP BY p.object_id, p.index_id
		) sz ON sz.object_id = i.object_id AND sz.index_id = i.index_id
		WHERE i.name IS NOT NULL
		  AND i.index_id > 0
		  AND OBJECTPROPERTY(i.object_id, 'IsUserTable') = 1
	`

	rows, err := dbq.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SqlServerIndexUsageRow
	var scanErrs int
	for rows.Next() {
		var r SqlServerIndexUsageRow
		if err := rows.Scan(&r.DBName, &r.SchemaName, &r.TableName, &r.IndexName, &r.SeeksTotal, &r.ScansTotal, &r.LookupsTotal, &r.UpdatesTotal, &r.LastUserSeek, &r.LastUserScan, &r.LastUserLookup, &r.IndexSizeMB, &r.IsUnique, &r.IsPK, &r.FillFactor); err != nil {
			scanErrs++
			if scanErrs == 1 {
				log.Printf("[Collector][SIH] CollectSQLServerIndexUsage row Scan error (subsequent rows omitted from log): %v", err)
			}
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 && scanErrs > 0 {
		return nil, fmt.Errorf("index usage: %d rows failed to scan (check driver/types vs DMV)", scanErrs)
	}
	return out, nil
}

// PersistSQLServerIndexUsageDeltas computes deltas and writes to Timescale monitor.index_usage_stats.
func PersistSQLServerIndexUsageDeltas(ctx context.Context, tl *hot.TimescaleLogger, serverID, dbName string, rows []SqlServerIndexUsageRow, capture time.Time) (inserted int, err error) {
	engine := "sqlserver"
	for _, r := range rows {
		// state key includes DBName from row (should match dbName argument)
		prev, err := tl.GetIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName)
		if err != nil {
			return inserted, fmt.Errorf("get index usage state: %w", err)
		}

		// First observation: seed state and emit one bootstrap row (delta vs implicit 0) so the
		// dashboard is not empty until a second 15m collection completes.
		if prev == nil {
			ds, ok1 := storageindex.Delta(r.SeeksTotal, 0)
			dsc, ok2 := storageindex.Delta(r.ScansTotal, 0)
			dl, ok3 := storageindex.Delta(r.LookupsTotal, 0)
			du, ok4 := storageindex.Delta(r.UpdatesTotal, 0)
			if ok1 && ok2 && ok3 && ok4 {
				stat := models.IndexUsageStat{
					Time:        capture.UTC(),
					Engine:      engine,
					ServerID:    serverID,
					DBName:      r.DBName,
					SchemaName:  r.SchemaName,
					TableName:   r.TableName,
					IndexName:   r.IndexName,
					Seeks:       ds,
					Scans:       dsc,
					Lookups:     dl,
					Updates:     du,
					IndexSizeMB: r.IndexSizeMB,
					IsUnique:    r.IsUnique,
					IsPK:        r.IsPK,
					FillFactor:  r.FillFactor,
				}
				if r.LastUserSeek.Valid {
					t := r.LastUserSeek.Time.UTC()
					stat.LastUserSeek = &t
				}
				if r.LastUserScan.Valid {
					t := r.LastUserScan.Time.UTC()
					stat.LastUserScan = &t
				}
				if r.LastUserLookup.Valid {
					t := r.LastUserLookup.Time.UTC()
					stat.LastUserLookup = &t
				}
				if err := tl.InsertIndexUsageStat(ctx, stat); err != nil {
					log.Printf("[Collector] index_usage_stats bootstrap insert failed: %v", err)
				} else {
					inserted++
				}
			}
			if err := tl.UpsertIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName, r.SeeksTotal, r.ScansTotal, r.LookupsTotal, r.UpdatesTotal); err != nil {
				return inserted, fmt.Errorf("seed index usage state: %w", err)
			}
			continue
		}

		ds, ok1 := storageindex.Delta(r.SeeksTotal, prev.SeeksTotal)
		dsc, ok2 := storageindex.Delta(r.ScansTotal, prev.ScansTotal)
		dl, ok3 := storageindex.Delta(r.LookupsTotal, prev.LookupsTotal)
		du, ok4 := storageindex.Delta(r.UpdatesTotal, prev.UpdatesTotal)

		// If any counter reset, refresh state and skip delta row.
		if !(ok1 && ok2 && ok3 && ok4) {
			_ = tl.UpsertIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName, r.SeeksTotal, r.ScansTotal, r.LookupsTotal, r.UpdatesTotal)
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
			Seeks:       ds,
			Scans:       dsc,
			Lookups:     dl,
			Updates:     du,
			IndexSizeMB: r.IndexSizeMB,
			IsUnique:    r.IsUnique,
			IsPK:        r.IsPK,
			FillFactor:  r.FillFactor,
		}
		if r.LastUserSeek.Valid {
			t := r.LastUserSeek.Time.UTC()
			stat.LastUserSeek = &t
		}
		if r.LastUserScan.Valid {
			t := r.LastUserScan.Time.UTC()
			stat.LastUserScan = &t
		}
		if r.LastUserLookup.Valid {
			t := r.LastUserLookup.Time.UTC()
			stat.LastUserLookup = &t
		}
		if err := tl.InsertIndexUsageStat(ctx, stat); err != nil {
			log.Printf("[Collector] index_usage_stats insert failed: %v", err)
			continue
		}
		inserted++

		if err := tl.UpsertIndexUsageState(ctx, engine, serverID, r.DBName, r.SchemaName, r.TableName, r.IndexName, r.SeeksTotal, r.ScansTotal, r.LookupsTotal, r.UpdatesTotal); err != nil {
			return inserted, fmt.Errorf("update index usage state: %w", err)
		}
	}
	return inserted, nil
}
