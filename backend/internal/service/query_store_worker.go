// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for SQL Server Query Store metrics.
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

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

func (s *MetricsService) StartQueryStoreCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server Query Store", 15*time.Minute)
	slog.Info("[QueryStore] Starting background collector (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Query Store", 15*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[QueryStore] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.collectQueryStoreStats(ctx)
				}
			}
		}
	}()
}

func (s *MetricsService) collectQueryStoreStats(ctx context.Context) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		serverID := inst.ServerID

		// Fetch stats (handles multiple DBs internally)
		stats, err := s.MsRepo.FetchQueryStoreStats(ctx, inst.Name)
		if err == nil && len(stats) > 0 {
			var hotRows []hot.QueryStoreStatsRow
			for _, st := range stats {
				hotRows = append(hotRows, hot.QueryStoreStatsRow{
					CaptureTimestamp:  time.Now().UTC(),
					ServerID:          serverID,
					ServerName:        inst.Name,
					DatabaseName:      st.DatabaseName,
					QueryHash:         st.QueryHash,
					QueryText:         st.QueryText,
					PlanID:            st.PlanID,
					IntervalID:        st.IntervalID,
					Executions:        st.Executions,
					AvgDurationMs:     st.AvgDurationMs,
					AvgCpuMs:          st.AvgCpuMs,
					AvgLogicalReads:   st.AvgLogicalReads,
					TotalCpuMs:        st.TotalCpuMs,
					LastExecutionTime: st.LastExecutionTime,
				})
			}
			_ = s.tsLogger.LogQueryStoreStats(ctx, serverID, hotRows)
		} else if err != nil {
			slog.Error("[QueryStore] ERROR", "target", inst.Name, "err", err)
		}
	}
}
