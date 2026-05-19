// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background collector for SQL Server Storage & Index Health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"fmt"
	"log/slog"
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) StartSqlServerStorageHistoryCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server Storage History", 6*time.Hour)
	slog.Info("[SQLServerStorage] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		// Run once immediately
		if interval > 0 {
			s.collectSqlServerStorageStats(ctx)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Storage History", 6*time.Hour)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[SQLServerStorage] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.collectSqlServerStorageStats(ctx)
				}
			}
		}
	}()
}

func (s *MetricsService) collectSqlServerStorageStats(ctx context.Context) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		serverID := inst.ServerID

		// Discover databases for this instance
		db, ok := s.MsRepo.GetConn(inst.Name)
		if !ok {
			continue
		}

		var databases []string
		rows, err := db.Query("SELECT name FROM sys.databases WHERE state = 0 AND name NOT IN ('master','tempdb','model','msdb')")
		if err == nil {
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err == nil {
					databases = append(databases, name)
				}
			}
			rows.Close()
		}

		for _, dbName := range databases {
			// Table Size History
			stats, err := s.MsRepo.FetchTableSizeStats(ctx, inst.Name, dbName)
			if err != nil {
				slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: FetchTableSizeStats error: %v", inst.Name, dbName, err))
				_ = s.tsLogger.LogCollectorError(ctx, serverID, "Permission error or query failure: "+err.Error())
				continue
			}
			if len(stats) > 0 {
				var hotRows []hot.TableSizeHistoryRow
				now := time.Now().UTC()
				for _, st := range stats {
					row := hot.TableSizeHistoryRow{
						CaptureTimestamp: now,
						ServerID:         serverID,
						DatabaseName:     dbName,
						SchemaName:       st.SchemaName,
						TableName:        st.TableName,
						RowCount:         st.RowCount,
						TotalMB:          st.TotalMB,
						DataMB:           st.DataMB,
						IndexMB:          st.IndexMB,
					}
					hotRows = append(hotRows, row)

					// Also populate unified monitor table
					_ = s.tsLogger.InsertTableSizeHistory(ctx, models.TableSizeHistory{
						Time:        now,
						Engine:      "sqlserver",
						ServerID:    serverID,
						DBName:      dbName,
						SchemaName:  st.SchemaName,
						TableName:   st.TableName,
						TableSizeMB: st.DataMB,
						IndexSizeMB: st.IndexMB,
						RowCount:    st.RowCount,
					})
				}
				_ = s.tsLogger.LogTableSizeHistoryWithChangeDetection(ctx, serverID, hotRows)
			}

			// Index Usage History
			idxStats, err := s.MsRepo.CollectSQLServerIndexUsageMetrics(ctx, inst.Name, dbName)
			if err != nil {
				slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: IndexUsage error: %v", inst.Name, dbName, err))
				_ = s.tsLogger.LogCollectorError(ctx, serverID, "Index usage collection error: "+err.Error())
				continue
			}
			if len(idxStats) > 0 {
				now := time.Now().UTC()
				for _, st := range idxStats {
					var lastSeek *time.Time
					if lus, ok := st["last_user_seek"].(sql.NullTime); ok && lus.Valid {
						t := lus.Time
						lastSeek = &t
					}
					_ = s.tsLogger.InsertIndexUsageStat(ctx, models.IndexUsageStat{
						Time:         now,
						Engine:       "sqlserver",
						ServerID:     serverID,
						DBName:       dbName,
						SchemaName:   st["schema_name"].(string),
						TableName:    st["table_name"].(string),
						IndexName:    st["index_name"].(string),
						Seeks:        st["user_seeks"].(int64),
						Scans:        st["user_scans"].(int64),
						Lookups:      st["user_lookups"].(int64),
						Updates:      st["user_updates"].(int64),
						IndexSizeMB:  st["index_size_mb"].(float64),
						IsPK:         st["is_pk"].(bool),
						IsUnique:     st["is_unique"].(bool),
						LastUserSeek: lastSeek,
					})
				}
			}

			// Table Usage Stats — seq/idx scans, rows read/modified per table.
			// Uses sys.dm_db_index_operational_stats (TVF) for row-level operation counters;
			// sys.dm_db_index_usage_stats only has user_seeks/scans/lookups/updates, not leaf counts.
			tableUsageRows, tableUsageErr := db.QueryContext(ctx, `
				/* SQL_OPTIMA */
				SELECT
					ISNULL(s.name, 'dbo')                                             AS schema_name,
					ISNULL(t.name, 'Unknown')                                          AS table_name,
					ISNULL(SUM(ios.range_scan_count), 0)                               AS seq_scans,
					ISNULL(SUM(ios.singleton_lookup_count), 0)                         AS idx_scans,
					ISNULL(SUM(ios.leaf_insert_count + ios.leaf_delete_count +
					           ios.leaf_update_count + ios.range_scan_count +
					           ios.singleton_lookup_count), 0)                         AS rows_read,
					ISNULL(SUM(ios.leaf_insert_count + ios.leaf_delete_count +
					           ios.leaf_update_count), 0)                              AS rows_modified,
					ISNULL(SUM(a.total_pages) * 8.0 / 1024.0, 0)                      AS table_size_mb,
					ISNULL(SUM(CASE WHEN i.index_id > 1 THEN a.total_pages ELSE 0 END) * 8.0 / 1024.0, 0) AS index_size_mb,
					ISNULL(MAX(p.rows), 0)                                             AS row_count
				FROM sys.tables t WITH (NOLOCK)
				JOIN sys.schemas s WITH (NOLOCK) ON t.schema_id = s.schema_id
				LEFT JOIN sys.indexes i WITH (NOLOCK) ON t.object_id = i.object_id
				LEFT JOIN sys.dm_db_index_operational_stats(DB_ID(), NULL, NULL, NULL) ios
					ON i.object_id = ios.object_id AND i.index_id = ios.index_id
				LEFT JOIN sys.partitions p WITH (NOLOCK)
					ON i.object_id = p.object_id AND i.index_id = p.index_id
				LEFT JOIN sys.allocation_units a WITH (NOLOCK)
					ON p.partition_id = a.container_id
				WHERE t.is_ms_shipped = 0
				GROUP BY s.name, t.name
				HAVING SUM(a.total_pages) > 0
				ORDER BY table_size_mb DESC
			`)
			if tableUsageErr != nil {
				slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: TableUsageStats error: %v", inst.Name, dbName, tableUsageErr))
				_ = s.tsLogger.LogCollectorError(ctx, serverID, "Table usage stats collection error: "+tableUsageErr.Error())
			} else {
				now := time.Now().UTC()
				for tableUsageRows.Next() {
					var schemaName, tableName string
					var seqScans, idxScans, rowsRead, rowsModified, rowCount int64
					var tableSizeMB, indexSizeMB float64
					if err := tableUsageRows.Scan(&schemaName, &tableName, &seqScans, &idxScans, &rowsRead, &rowsModified, &tableSizeMB, &indexSizeMB, &rowCount); err != nil {
						continue
					}
					_ = s.tsLogger.InsertTableUsageStat(ctx, models.TableUsageStat{
						Time:         now,
						Engine:       "sqlserver",
						ServerID:     serverID,
						DBName:       dbName,
						SchemaName:   schemaName,
						TableName:    tableName,
						SeqScans:     seqScans,
						IdxScans:     idxScans,
						RowsRead:     rowsRead,
						RowsModified: rowsModified,
						TableSizeMB:  tableSizeMB,
						IndexSizeMB:  indexSizeMB,
						RowCount:     rowCount,
					})
				}
				tableUsageRows.Close()
			}
		}
	}
}
