// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker that periodically collects query regression and
//
//	plan instability data from SQL Server Query Store, and watched-query
//	snapshots, persisting results to TimescaleDB.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"log"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

const (
	queryAnalysisInterval = 30 * time.Minute
	watchedQueryInterval  = 5 * time.Minute
)

// StartQueryAnalysisCollector runs every 30 min, fetching regressions and plan instability
// from SQL Server Query Store for each configured instance and persisting to TimescaleDB.
func (s *MetricsService) StartQueryAnalysisCollector(ctx context.Context) {
	ticker := time.NewTicker(queryAnalysisInterval)
	defer ticker.Stop()

	// Run immediately on startup.
	s.collectQueryAnalysisData(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collectQueryAnalysisData(ctx)
		}
	}
}

func (s *MetricsService) collectQueryAnalysisData(ctx context.Context) {
	if s.MsRepo == nil || s.tsLogger == nil {
		return
	}

	now := time.Now().UTC()

	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}

		// Regressions
		regs, err := s.MsRepo.FetchQueryRegressions(inst.Name)
		if err != nil {
			log.Printf("[QueryAnalysis] FetchQueryRegressions(%s): %v", inst.Name, err)
		} else if len(regs) > 0 {
			rows := make([]hot.SqlServerQueryRegressionRow, len(regs))
			for i, r := range regs {
				rows[i] = hot.SqlServerQueryRegressionRow{
					CaptureTime: now, InstanceName: inst.Name, DatabaseName: r.DatabaseName,
					QueryHash: r.QueryHash, QueryText: r.QueryText, RegressionType: r.RegressionType,
					PreviousAvg: r.PreviousAvg, CurrentAvg: r.CurrentAvg,
					PercentChange: r.PercentChange, PlanChanged: r.PlanChanged,
				}
			}
			if err := s.tsLogger.LogSqlServerQueryRegressions(ctx, rows); err != nil {
				log.Printf("[QueryAnalysis] LogSqlServerQueryRegressions(%s): %v", inst.Name, err)
			}
		}

		// Plan instability
		pis, err := s.MsRepo.FetchPlanInstability(inst.Name)
		if err != nil {
			log.Printf("[QueryAnalysis] FetchPlanInstability(%s): %v", inst.Name, err)
		} else if len(pis) > 0 {
			rows := make([]hot.SqlServerPlanInstabilityRow, len(pis))
			for i, p := range pis {
				rows[i] = hot.SqlServerPlanInstabilityRow{
					CaptureTime: now, InstanceName: inst.Name, DatabaseName: p.DatabaseName,
					QueryHash: p.QueryHash, QueryText: p.QueryText,
					PlanCount: p.PlanCount, LastExecutionTime: p.LastExecutionTime,
				}
			}
			if err := s.tsLogger.LogSqlServerPlanInstability(ctx, rows); err != nil {
				log.Printf("[QueryAnalysis] LogSqlServerPlanInstability(%s): %v", inst.Name, err)
			}
		}
	}
}

// StartWatchedQueryCollector runs every 5 min, collecting current stats for all
// watched queries from SQL Server Query Store and persisting snapshot rows.
func (s *MetricsService) StartWatchedQueryCollector(ctx context.Context) {
	ticker := time.NewTicker(watchedQueryInterval)
	defer ticker.Stop()

	// Run immediately on startup.
	s.collectWatchedQuerySnapshots(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collectWatchedQuerySnapshots(ctx)
		}
	}
}

func (s *MetricsService) collectWatchedQuerySnapshots(ctx context.Context) {
	if s.MsRepo == nil || s.tsLogger == nil {
		return
	}

	now := time.Now().UTC()

	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}

		wqs, err := s.tsLogger.ListSqlServerWatchedQueries(ctx, inst.Name)
		if err != nil {
			log.Printf("[WatchedQuery] ListSqlServerWatchedQueries(%s): %v", inst.Name, err)
			continue
		}
		if len(wqs) == 0 {
			continue
		}

		var snapRows []hot.SqlServerWatchedSnapshotRow
		for _, wq := range wqs {
			if wq.QueryHash == "" || wq.DatabaseName == "" {
				continue
			}
			// Use the stored database_name for targeted Query Store lookup
			snap, err := s.MsRepo.FetchWatchedQueryStats(ctx, inst.Name, wq.DatabaseName, wq.QueryHash)
			if err != nil {
				log.Printf("[WatchedQuery] FetchWatchedQueryStats(%s, %s, %s): %v", inst.Name, wq.DatabaseName, wq.QueryHash, err)
				continue
			}
			if snap == nil {
				continue
			}

			// If registry is missing query text, update it now that we have it from Query Store
			if wq.QueryText == "" && snap.QueryText != "" {
				_ = s.tsLogger.UpdateSqlServerWatchedQueryText(ctx, wq.ID, snap.QueryText)
			}

			// Also fetch wait stats for this query hash specifically
			waitStats, _ := s.MsRepo.FetchQueryWaitStats(ctx, inst.Name, wq.DatabaseName, wq.QueryHash)

			log.Printf("[WatchedQuery] Captured stats for %s: %d executions", wq.Name, snap.Executions)
			snapRows = append(snapRows, hot.SqlServerWatchedSnapshotRow{
				SnapshotTime:      now,
				WatchedID:         wq.ID,
				InstanceName:      inst.Name,
				Executions:        snap.Executions,
				AvgDurationMs:     snap.AvgDurationMs,
				AvgCpuMs:          snap.AvgCpuMs,
				AvgReads:          snap.AvgReads,
				TotalDurationMs:   snap.TotalDurationMs,
				TotalCpuMs:        snap.TotalCpuMs,
				PlanCount:         snap.PlanCount,
				LastExecutionTime: snap.LastExecutionTime,
				QueryPlan:         snap.QueryPlan,
				WaitStats:         waitStats,
			})
		}

		if len(snapRows) > 0 {
			if err := s.tsLogger.LogSqlServerWatchedQuerySnapshot(ctx, snapRows); err != nil {
				log.Printf("[WatchedQuery] LogSqlServerWatchedQuerySnapshot(%s): %v", inst.Name, err)
			}
		}
	}
}
