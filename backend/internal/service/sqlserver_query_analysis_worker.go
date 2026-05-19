// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for SQL Server Query Analysis Dashboard —
//
//	detects regressions and plan instability, stores snapshots for watched queries.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"fmt"
	"log/slog"
	"context"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) StartQueryAnalysisCollector(ctx context.Context) {
	// Query analysis is moderate weight; run every 15-30 mins
	interval := s.GetCollectorInterval(ctx, "SQL Server Query Analysis", 30*time.Minute)
	slog.Info("[QueryAnalysis] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		// Run once at start
		if interval > 0 {
			s.collectQueryAnalysisData(ctx)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Query Analysis", 30*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[QueryAnalysis] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.collectQueryAnalysisData(ctx)
				}
			}
		}
	}()
}

func (s *MetricsService) collectQueryAnalysisData(ctx context.Context) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		serverID := inst.ServerID

		// Regressions
		regStats, err := s.MsRepo.FetchQueryRegressions(ctx, inst.Name)
		if err == nil && len(regStats) > 0 {
			var regressions []hot.SqlServerQueryRegressionRow
			now := time.Now().UTC()
			for _, r := range regStats {
				regressions = append(regressions, hot.SqlServerQueryRegressionRow{
					CaptureTime:    now,
					ServerID:       serverID,
					DatabaseName:   r.DatabaseName,
					QueryHash:      r.QueryHash,
					QueryText:      r.QueryText,
					RegressionType: r.RegressionType,
					PreviousAvg:    r.PreviousAvg,
					CurrentAvg:     r.CurrentAvg,
					PercentChange:  r.PercentChange,
					PlanChanged:    r.PlanChanged,
				})
			}
			_ = s.tsLogger.LogSqlServerQueryRegressions(ctx, regressions)
		}

		// Plan Instability
		instabStats, err := s.MsRepo.FetchPlanInstability(ctx, inst.Name)
		if err == nil && len(instabStats) > 0 {
			var instability []hot.SqlServerPlanInstabilityRow
			now := time.Now().UTC()
			for _, r := range instabStats {
				instability = append(instability, hot.SqlServerPlanInstabilityRow{
					CaptureTime:       now,
					ServerID:          serverID,
					DatabaseName:      r.DatabaseName,
					QueryHash:         r.QueryHash,
					QueryText:         r.QueryText,
					PlanCount:         r.PlanCount,
					LastExecutionTime: r.LastExecutionTime,
				})
			}
			_ = s.tsLogger.LogSqlServerPlanInstability(ctx, instability)
		}
	}
}

func (s *MetricsService) StartWatchedQueryCollector(ctx context.Context) {
	// Watched query snapshots; run every 15 mins
	interval := s.GetCollectorInterval(ctx, "SQL Server Watched Query Snapshot", 15*time.Minute)
	slog.Info("[WatchedQueries] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		// Run once at start
		if interval > 0 {
			s.collectWatchedQuerySnapshots(ctx)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Watched Query Snapshot", 15*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[WatchedQueries] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.collectWatchedQuerySnapshots(ctx)
				}
			}
		}
	}()
}

func (s *MetricsService) collectWatchedQuerySnapshots(ctx context.Context) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		serverID := inst.ServerID
		watched, err := s.tsLogger.ListSqlServerWatchedQueries(ctx, serverID)
		if err != nil {
			continue
		}

		var snapshots []hot.SqlServerWatchedSnapshotRow
		now := time.Now().UTC()

		for _, q := range watched {
			stats, err := s.MsRepo.FetchWatchedQueryStats(ctx, inst.Name, q.DatabaseName, q.QueryHash)
			if err != nil {
				slog.Error(fmt.Sprintf("[WatchedQueries] FetchWatchedQueryStats error for hash %s db %s: %v", q.QueryHash, q.DatabaseName, err))
				continue
			}
			if stats == nil {
				continue
			}

			snap := hot.SqlServerWatchedSnapshotRow{
				WatchedID:         q.ID,
				ServerID:          serverID,
				CaptureTimestamp:  now,
				Executions:        stats.Executions,
				AvgDurationMs:     stats.AvgDurationMs,
				AvgCpuMs:          stats.AvgCpuMs,
				AvgReads:          stats.AvgReads,
				TotalDurationMs:   stats.TotalDurationMs,
				TotalCpuMs:        stats.TotalCpuMs,
				PlanCount:         stats.PlanCount,
				LastExecutionTime: stats.LastExecutionTime,
				QueryPlan:         stats.QueryPlan,
				QueryText:         stats.QueryText,
			}
			snapshots = append(snapshots, snap)
		}

		if len(snapshots) > 0 {
			_ = s.tsLogger.LogSqlServerWatchedQuerySnapshot(ctx, snapshots)
		}
	}
}
