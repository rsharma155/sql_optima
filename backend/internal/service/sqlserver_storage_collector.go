/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Dedicated SQL Server Storage & Index Health history collector.
 *          Captures daily snapshots of DB/Table/Index sizes and usage for TimescaleDB.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

package service

import (
	"context"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/collectors"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// StartSqlServerStorageHistoryCollector starts the background task to collect storage snapshots.
func (s *MetricsService) StartSqlServerStorageHistoryCollector(ctx context.Context) {
	interval := s.FetchInterval(ctx, "SQL Server Storage", 24*time.Hour)
	log.Printf("[Collector][SQLSERVER-Storage] History collector started (interval: %v)", interval)

	// Ticker for storage collection
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial run after short delay
	time.AfterFunc(1*time.Minute, func() {
		s.RunSqlServerStorageSnapshotCollection(ctx)
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunSqlServerStorageSnapshotCollection(ctx)

			// Refresh interval from DB
			newInterval := s.FetchInterval(ctx, "SQL Server Storage", 24*time.Hour)
			if newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
			}
		}
	}
}

// RunSqlServerStorageSnapshotCollection performs a full sweep of monitored SQL instances.
func (s *MetricsService) RunSqlServerStorageSnapshotCollection(ctx context.Context) {
	if s.tsLogger == nil {
		return
	}

	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}

		log.Printf("[Collector][SQLSERVER-Storage] Collecting snapshots for instance: %s", inst.Name)

		s.sihMu.Lock()
		lastDefTime := s.sihLastDefsDaily[inst.Name]
		s.sihMu.Unlock()

		// 1. Database Storage Metrics
		dbMetrics, err := s.MsRepo.CollectSQLServerStorageMetrics(inst.Name)
		if err == nil {
			var rows []hot.DBStorageHistoryRow
			now := time.Now().UTC()
			for _, m := range dbMetrics {
				rows = append(rows, hot.DBStorageHistoryRow{
					SnapshotTime: now,
					ServerName:   inst.Name,
					InstanceName: inst.Name,
					DatabaseName: m["database_name"].(string),
					TotalSizeMB:  m["total_size_mb"].(float64),
					DataSizeMB:   m["data_size_mb"].(float64),
					LogSizeMB:    m["log_size_mb"].(float64),
				})
			}
			_ = s.tsLogger.LogDBStorageHistory(ctx, rows)
		}

		// 2. Table-level Metrics (Iterate through user databases)
		databases, _ := s.MsRepo.ListSQLServerUserDatabases(inst.Name)
		capture := time.Now().UTC()
		for _, db := range databases {
			// A: Table Sizes
			tblMetrics, err := s.MsRepo.CollectSQLServerTableSizeMetrics(inst.Name, db)
			if err == nil {
				var rows []hot.TableSizeHistoryRow
				for _, m := range tblMetrics {
					rows = append(rows, hot.TableSizeHistoryRow{
						SnapshotTime: capture,
						ServerName:   inst.Name,
						InstanceName: inst.Name,
						DatabaseName: db,
						SchemaName:   m["schema_name"].(string),
						TableName:    m["table_name"].(string),
						RowCount:     m["row_count"].(int64),
						TotalMB:      m["total_mb"].(float64),
						DataMB:       m["data_mb"].(float64),
						IndexMB:      m["index_mb"].(float64),
					})
				}
				_ = s.tsLogger.LogTableSizeHistory(ctx, rows)
			}

			// B: Table Structure Risks
			structMetrics, err := s.MsRepo.CollectSQLServerTableStructureMetrics(inst.Name, db)
			if err == nil {
				var rows []hot.TableStructureHistoryRow
				for _, m := range structMetrics {
					rows = append(rows, hot.TableStructureHistoryRow{
						SnapshotTime:      capture,
						ServerName:        inst.Name,
						InstanceName:      inst.Name,
						DatabaseName:      db,
						SchemaName:        m["schema_name"].(string),
						TableName:         m["table_name"].(string),
						HasClusteredIndex: m["has_clustered_index"].(bool),
						HasPrimaryKey:     m["has_primary_key"].(bool),
					})
				}
				_ = s.tsLogger.LogTableStructureHistory(ctx, rows)
			}

			// C: Index Usage Stats (Deltas for SIH charts)
			idxRows, ierr := collectors.CollectSQLServerIndexUsage(ctx, s.MsRepo.AsQueryer(inst.Name, db))
			if ierr == nil {
				_, _ = collectors.PersistSQLServerIndexUsageDeltas(ctx, s.tsLogger, inst.Name, db, idxRows, capture)
			}

			// D: Index Definitions (Daily)
			defRows, derr := collectors.CollectSQLServerIndexDefinitions(ctx, s.MsRepo.AsQueryer(inst.Name, db), lastDefTime)
			if derr == nil {
				_, _ = collectors.PersistSQLServerIndexDefinitions(ctx, s.tsLogger, inst.Name, defRows, capture)
			}

			// E: Index Fragmentation (Daily)
			fragMetrics, ferr := s.MsRepo.CollectSQLServerIndexFragmentationMetrics(inst.Name, db)
			if ferr == nil {
				var rows []hot.IndexFragHistoryRow
				for _, m := range fragMetrics {
					rows = append(rows, hot.IndexFragHistoryRow{
						SnapshotTime:        capture,
						ServerName:          inst.Name,
						InstanceName:        inst.Name,
						DatabaseName:        db,
						SchemaName:          m["schema_name"].(string),
						TableName:           m["table_name"].(string),
						IndexName:           m["index_name"].(string),
						AvgFragmentationPct: m["avg_fragmentation_pct"].(float64),
						PageCount:           m["page_count"].(int64),
					})
				}
				_ = s.tsLogger.LogIndexFragmentationHistory(ctx, rows)
			}
		}

		// Update last run time for index definitions to optimize next run
		s.sihMu.Lock()
		s.sihLastDefsDaily[inst.Name] = capture
		s.sihMu.Unlock()

		// 3. Global SIH Refreshes
		_, _ = s.tsLogger.RefreshIndexUnusedCandidatesDaily(ctx, "sqlserver", inst.Name, capture, 100)
	}
}
