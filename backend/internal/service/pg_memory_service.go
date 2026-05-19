// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background worker for PostgreSQL memory intelligence snapshots.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *MetricsService) CollectPostgresMemoryIntelligence(ctx context.Context) {
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "postgres" {
			continue
		}

		// 1. Host memory — only available when the os_collector agent is running.
		//    CollectPgHostMemory returns an error when no real OS data is available,
		//    which keeps host_memory_samples clean (no fake hardcoded values).
		hostMem, err := s.PgRepo.CollectPgHostMemory(inst.Name)
		if err == nil && hostMem != nil {
			_ = s.tsLogger.LogPgHostMemory(ctx, hostMem)
		}

		// 2. PG-internal memory stats (cache hit, connections, temp spill, bgwriter).
		memStats, err := s.PgRepo.CollectPgMemoryStats(ctx, inst.Name)
		if err == nil {
			_ = s.tsLogger.LogPgMemoryStats(ctx, memStats)
			// Compute derived metrics (cache hit ratio, spill rate, pressure) immediately
			// after writing the raw sample so the dashboard query finds populated data.
			_ = s.tsLogger.ComputeAndLogPgMemoryDerived(ctx, memStats.ServerID)
		}

		// 3. Memory configuration GUCs (shared_buffers, work_mem, etc.).
		components, err := s.PgRepo.CollectPgMemoryComponents(ctx, inst.Name)
		if err == nil {
			_ = s.tsLogger.LogPgMemoryComponents(ctx, components)
		}
	}
}

func (s *MetricsService) GetPgMemoryTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) (map[string]interface{}, error) {
	return s.tsLogger.GetPgMemoryTimeSeries(ctx, serverID, from, to)
}
