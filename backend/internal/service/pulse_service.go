// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Multi-threaded "Pulse" collector for high-frequency (Tier 1-3) metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/config"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type PulseService struct {
	cfg        *config.Config
	msRepo     *repository.SqlServerRepository
	pgRepo     *repository.PgRepository
	tsLogger   *hot.TimescaleLogger
	stopChan   chan struct{}
	wg         sync.WaitGroup
	metricsSvc *MetricsService
}

func NewPulseService(cfg *config.Config, ms *repository.SqlServerRepository, pg *repository.PgRepository, ts *hot.TimescaleLogger, metricsSvc *MetricsService) *PulseService {
	return &PulseService{
		cfg:        cfg,
		msRepo:     ms,
		pgRepo:     pg,
		tsLogger:   ts,
		stopChan:   make(chan struct{}),
		metricsSvc: metricsSvc,
	}
}

func (s *PulseService) Start(ctx context.Context) {
	// Use a default name, but we check per-type in the loop if needed.
	// For the main ticker, we'll use a generic or the most frequent one.
	intervalName := "sqlserver_sessions_activity_pulse"
	interval := s.metricsSvc.GetCollectorInterval(ctx, intervalName, 5*time.Second)
	slog.Info("[PulseService] Starting collection loop (%v interval)", "val", interval)
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			case <-ticker.C:
				// Dynamic interval check: if config changed, reset ticker
				newInterval := s.metricsSvc.GetCollectorInterval(ctx, intervalName, 5*time.Second)
				if newInterval != interval && newInterval > 0 {
					slog.Info("[PulseService] Frequency changed from", "arg1", interval, "arg2", newInterval)
					interval = newInterval
					ticker.Reset(interval)
				}

				if interval > 0 {
					s.runSweep(ctx)
				}
			}
		}
	}()
}

func (s *PulseService) runSweep(ctx context.Context) {
	for _, inst := range s.cfg.Instances {
		s.wg.Add(1)
		go func(instance config.Instance) {
			defer s.wg.Done()
			if strings.ToLower(instance.Type) == "sqlserver" {
				s.runSQLServerPulse(ctx, instance)
			} else if strings.ToLower(instance.Type) == "postgres" {
				s.runPgPulse(ctx, instance)
			}
		}(inst)
	}
	s.wg.Wait()
}

func (s *PulseService) runSQLServerPulse(ctx context.Context, inst config.Instance) {
	start := time.Now()
	serverID := inst.ServerID

	// Always read from metricsSvc so reloaded repos (after AddServer) are picked up.
	msRepo := s.metricsSvc.MsRepo
	if msRepo == nil {
		return
	}

	// 1. Live sessions (Tier 1)
	db, ok := msRepo.GetConn(inst.Name)
	if !ok {
		return
	}

	rows, err := repository.CollectActiveQueries(ctx, db)
	if err == nil {
		// Map and Log
		var stagingRows []map[string]interface{}
		for _, r := range rows {
			stagingRows = append(stagingRows, map[string]interface{}{
				"session_id":            r.SessionID,
				"status":                r.Status,
				"wait_type":             r.WaitType,
				"cpu_time":              r.CPUTimeMs,
				"wait_time":             r.WaitTimeMs,
				"last_wait_type":        r.WaitType,
				"database_name":         r.DatabaseName,
				"query_text":            r.QueryText,
				"login_name":            r.LoginName,
				"host_name":             r.HostName,
				"program_name":          r.ProgramName,
				"command":               r.Command,
				"total_elapsed_time":    r.TotalElapsedTimeMs,
				"reads":                 r.Reads,
				"writes":                r.Writes,
				"granted_memory_mb":     r.GrantedMemoryMB,
				"row_count":             r.RowCount,
				"percent_complete":      r.PercentComplete,
			})
		}
		_ = s.tsLogger.LogStagingTier1SQLServer(ctx, serverID, stagingRows)
	}

	// 2. Locks (Tier 1) — write to staging and to sqlserver_blocking_snapshots
	locks, err := repository.CollectBlockingLocks(ctx, db)
	if err == nil {
		var lockRows []map[string]interface{}
		var snapshots []models.SQLServerBlockingSnapshot
		now := time.Now().UTC()
		for _, l := range locks {
			lockRows = append(lockRows, map[string]interface{}{
				"session_id":          l.SessionID,
				"wait_duration_ms":    l.WaitTimeMs,
				"wait_type":           l.WaitType,
				"blocking_session_id": l.BlockingSessionID,
			})
			snapshots = append(snapshots, models.SQLServerBlockingSnapshot{
				Timestamp:         now,
				ServerID:          serverID,
				SessionID:         l.SessionID,
				BlockingSessionID: l.BlockingSessionID,
				WaitType:          l.WaitType,
				WaitDurationMs:    l.WaitTimeMs,
				DatabaseName:      l.DatabaseName,
				LoginName:         l.LoginName,
				HostName:          l.HostName,
				ProgramName:       l.ProgramName,
				QueryText:         l.QueryText,
				CpuTime:           l.CPUTimeMs,
			})
		}
		_ = s.tsLogger.LogStagingTier1SQLServerLocks(ctx, serverID, lockRows)
		if len(snapshots) > 0 {
			_ = s.tsLogger.LogSQLServerBlockingSnapshots(ctx, serverID, snapshots)
		}
	}

	// 3. CPU, Memory, and System KPIs (Tier 2)
	kpis, startTime, err := msRepo.FetchHealthV2KPIs(ctx, inst.Name)
	if err == nil {
		uptimeSeconds := int64(time.Since(startTime).Seconds())
		_ = s.tsLogger.LogSqlServerHealthV2KPIs(ctx, serverID, kpis, uptimeSeconds)

		// Also log to the legacy sqlserver_metrics for compatibility with some dashboard widgets
		metricsMap := map[string]interface{}{
			"avg_cpu_load":     kpis.SqlCpuPct,
			"memory_usage":     0.0, // Placeholder if not directly in kpis
			"active_users":     kpis.UserConnections,
			"total_locks":      0,
			"deadlocks":        0,
			"blocked_sessions": kpis.BlockedSessions,
		}
		_ = s.tsLogger.LogSQLServerMetrics(ctx, serverID, metricsMap)
	}

	// 4. Wait Stats (Tier 2)
	cumulativeWaits, err := msRepo.FetchWaitStatsCumulative(ctx, inst.Name)
	if err == nil && len(cumulativeWaits) > 0 {
		_ = s.tsLogger.ComputeAndLogWaitDeltas(ctx, serverID, cumulativeWaits)
	}

	_ = s.tsLogger.LogCollectorRun(ctx, serverID, "sqlserver_pulse", "success", time.Since(start).Milliseconds())
}

func (s *PulseService) runPgPulse(ctx context.Context, inst config.Instance) {
	start := time.Now()
	serverID := inst.ServerID

	// Always read from metricsSvc so reloaded repos (after AddServer) are picked up.
	pgRepo := s.metricsSvc.PgRepo
	if pgRepo == nil {
		return
	}

	db, ok := pgRepo.GetConn(inst.Name)
	if !ok {
		slog.Info("[PulseService] No connection for", "val", inst.Name)
		return
	}

	// 1. Live sessions (Tier 1) - Populates monitor.pg_session_activity_ts
	if s.metricsSvc.PgObsCollector != nil {
		_ = s.metricsSvc.PgObsCollector.CollectSessionActivity(ctx, serverID, db)
		_ = s.metricsSvc.PgObsCollector.CollectWaitEventSummary(ctx, serverID, db)
		_ = s.metricsSvc.PgObsCollector.CollectDBLoad(ctx, serverID, db)
	}

	// 2. Snapshot and KPIs (Tier 2) - Populates postgres_snapshot_metrics, pg_instance_snapshot, etc.
	if s.metricsSvc.PgSnapshotCollector != nil {
		_ = s.metricsSvc.PgSnapshotCollector.Collect(ctx, inst)
	}

	_ = s.tsLogger.LogCollectorRun(ctx, serverID, "postgres_pulse", "success", time.Since(start).Milliseconds())
}

