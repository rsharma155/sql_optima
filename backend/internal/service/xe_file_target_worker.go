// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for SQL Server Extended Events (XE) file targets.
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
)

func (s *MetricsService) StartXEFileTargetWorker(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server Extended Events", 1*time.Minute)
	slog.Info("[XEWorker] Starting background worker (interval: %v)", "val", interval)

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Dynamic interval check
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Extended Events", 1*time.Minute)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[XEWorker] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.processXEFiles(ctx)
				}
			}
		}
	}()
}

func (s *MetricsService) processXEFiles(ctx context.Context) {
	if s.tsLogger == nil {
		return
	}

	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		serverID := inst.ServerID
		db, ok := s.MsRepo.GetConn(inst.Name)
		if !ok {
			continue
		}

		// Ensure the deadlock XE session exists and is running
		_ = s.MsRepo.EnsureDeadlockXESession(ctx, db)

		// Fetch deadlock events from the XE file target
		events, err := s.MsRepo.FetchDeadlockEvents(ctx, db)
		if err != nil {
			slog.Error("[XEWorker] Failed to fetch deadlock events", "instance", inst.Name, "err", err)
			continue
		}

		if len(events) > 0 {
			err = s.tsLogger.LogSQLServerDeadlockEvents(ctx, serverID, events)
			if err != nil {
				slog.Error("[XEWorker] Failed to log deadlock events", "instance", inst.Name, "err", err)
			} else {
				slog.Info("[XEWorker] Successfully logged deadlock events", "instance", inst.Name, "count", len(events))
			}
		}
	}
}
