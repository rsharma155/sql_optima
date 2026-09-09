// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background collector for SQL Server Storage & Index Health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) StartSqlServerStorageHistoryCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "sqlserver_storage_index_health", 5*time.Minute)
	if interval <= 0 {
		slog.Warn("[SQLServerStorage] Collector is disabled (interval=0)")
		interval = 24 * time.Hour
	}
	slog.Info("[SQLServerStorage] Starting background collector", "interval", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		if interval > 0 {
			s.collectSqlServerStorageStats(ctx)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "sqlserver_storage_index_health", 5*time.Minute)
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
	if s.tsLogger == nil || s.MsRepo == nil || s.Config == nil {
		return
	}
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
		slog.Info("[SQLServerStorage] Discovered databases", "instance", inst.Name, "count", len(databases))

		collectUsage := s.sihDue(serverID, "sqlserver_storage_index_health_index15m", s.GetCollectorInterval(ctx, "sqlserver_storage_index_health_index15m", 15*time.Minute))
		collectGrowth := s.sihDue(serverID, "sqlserver_storage_index_health_growth6h", s.GetCollectorInterval(ctx, "SQL Server Storage History", 6*time.Hour))
		if !collectUsage && !collectGrowth {
			continue
		}

		for _, dbName := range databases {
			if collectGrowth {
				// Table Size History
				stats, err := s.MsRepo.FetchTableSizeStats(ctx, inst.Name, dbName)
				if err != nil {
					slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: FetchTableSizeStats error: %v", inst.Name, dbName, err))
					_ = s.tsLogger.LogCollectorError(ctx, serverID, "Permission error or query failure: "+err.Error())
				} else if len(stats) > 0 {
					slog.Debug("[SQLServerStorage] Collected table size stats", "db", dbName, "tables", len(stats))
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
							Timestamp:   now,
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
			}

			if !collectUsage {
				continue
			}

			// Index Usage History - using delta logic to avoid double-counting cumulative counters.
			idxRows, err := collectors.CollectSQLServerIndexUsage(ctx, db, dbName)
			if err != nil {
				slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: IndexUsage collection error: %v", inst.Name, dbName, err))
				_ = s.tsLogger.LogCollectorError(ctx, serverID, "Index usage collection error: "+err.Error())
				continue
			}
			if len(idxRows) > 0 {
				inserted, pErr := collectors.PersistSQLServerIndexUsageDeltas(ctx, s.tsLogger, serverID, dbName, idxRows, time.Now())
				if pErr != nil {
					slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: IndexUsage persistence error: %v", inst.Name, dbName, pErr))
				} else if inserted > 0 {
					slog.Debug("[SQLServerStorage] Persisted index usage deltas", "count", inserted, "db", dbName)
				}
			}

			tblRows, tblErr := collectors.CollectSQLServerTableSizeSnapshot(ctx, db, dbName)
			if tblErr != nil {
				slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: TableUsageStats error: %v", inst.Name, dbName, tblErr))
				_ = s.tsLogger.LogCollectorError(ctx, serverID, "Table usage stats collection error: "+tblErr.Error())
			} else if len(tblRows) > 0 {
				if _, pErr := collectors.PersistSQLServerTableUsageDeltas(ctx, s.tsLogger, serverID, tblRows, time.Now()); pErr != nil {
					slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: TableUsage persistence error: %v", inst.Name, dbName, pErr))
				}
				if _, pErr := collectors.PersistSQLServerTableSizeHistory(ctx, s.tsLogger, serverID, tblRows, time.Now()); pErr != nil {
					slog.Error(fmt.Sprintf("[SQLServerStorage] %s/%s: TableSizeHistory persistence error: %v", inst.Name, dbName, pErr))
				}
			}
		}
	}
}
