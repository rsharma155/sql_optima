// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background collector for SQL Server Memory Intelligence metrics.
//          Calls FetchMemoryAnalyzerSnapshot and FetchBufferPoolByDB on each
//          SQL Server instance and persists results to TimescaleDB.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/repository"
)

// StartMemoryIntelligenceCollector collects SQL Server memory metrics into TimescaleDB.
// Runs every 60 seconds (or as configured). Covers:
//   - SQL memory used vs target, OS total/available
//   - Memory grants (pending, active, waiting)
//   - PLE, plan cache size, TempDB spill counters
//   - Buffer pool by database (top 20)
func (s *MetricsService) StartMemoryIntelligenceCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server Memory Intelligence", 60*time.Second)
	slog.Info("[MemoryCollector] Starting memory intelligence collector (interval: %v)", "val", interval)

	go func() {
		// First scrape immediately so dashboards populate within one cycle (ticker waits interval first).
		s.collectMemoryIntelligence(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Memory Intelligence", 60*time.Second)
				if newInterval != interval && newInterval > 0 {
					interval = newInterval
					ticker.Reset(interval)
				}
				s.collectMemoryIntelligence(ctx)
			}
		}
	}()
}

func (s *MetricsService) collectMemoryIntelligence(ctx context.Context) {
	if !s.IsTimescaleConnected() {
		return
	}
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		snapshot, err := s.MsRepo.FetchMemoryAnalyzerSnapshot(ctx, inst.ServerID, inst.Name)
		if err != nil && repository.IsMSSQLConnError(err) {
			// Reconnect and let the next tick retry with the fresh pool.
			_ = s.MsRepo.ReconnectInstance(ctx, inst.Name)
		}
		if err != nil {
			slog.Error("[MemoryCollector]", "target", inst.Name, "err", err)
			continue
		}
		if err := s.tsLogger.LogSQLServerMemoryMetrics(ctx, inst.ServerID, snapshot); err != nil {
			slog.Error("[MemoryCollector]", "target", inst.Name, "err", err)
		}

		bpRows, err := s.MsRepo.FetchBufferPoolByDB(ctx, inst.Name, 20)
		if err != nil {
			slog.Error("[MemoryCollector]", "target", inst.Name, "err", err)
			continue
		}
		if err := s.tsLogger.LogSQLServerBufferPoolByDB(ctx, inst.ServerID, bpRows); err != nil {
			slog.Error("[MemoryCollector]", "target", inst.Name, "err", err)
		}

		clerkRows, err := s.MsRepo.FetchMemoryClerks(ctx, inst.Name)
		if err != nil {
			slog.Error("[MemoryCollector] clerks", "target", inst.Name, "err", err)
			continue
		}
		if err := s.tsLogger.LogSqlServerMemoryClerksV2(ctx, inst.ServerID, clerkRows); err != nil {
			slog.Error("[MemoryCollector] clerks log", "target", inst.Name, "err", err)
		}
	}
}
