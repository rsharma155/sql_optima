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
	if s.xeDb == nil {
		return
	}

	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}

		serverID := inst.ServerID
		// Logic to poll XE files from target instance and persist to xeDb
		_ = serverID
	}
}
