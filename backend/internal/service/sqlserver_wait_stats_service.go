// Package service provides business logic and orchestration for metrics collection.
// sqlserver_wait_stats_service.go implements the shared collector for Wait Stats V2.
// Metadata:
//   - Feature: SQL Server Wait Stats V2
//   - Layer: Service (Orchestration)
package service

import (
	"log/slog"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/pkg/dashboard"
)

var (
	waitSnapshotMu sync.Mutex
	waitSnapshots  = make(map[uuid.UUID]map[string][3]int64)

	ioSnapshotMu sync.Mutex
	ioSnapshots  = make(map[uuid.UUID]map[string]ioSample)
)

type ioSample struct {
	CapturedAt time.Time
	Reads      int64
	ReadBytes  int64
	ReadStall  int64
	Writes     int64
	WriteBytes int64
	WriteStall int64
	FileType   string
}

func (s *MetricsService) getPrevIOSnapshot(id uuid.UUID) map[string]ioSample {
	ioSnapshotMu.Lock()
	defer ioSnapshotMu.Unlock()
	return ioSnapshots[id]
}

func (s *MetricsService) storePrevIOSnapshot(id uuid.UUID, snap map[string]ioSample) {
	ioSnapshotMu.Lock()
	defer ioSnapshotMu.Unlock()
	ioSnapshots[id] = snap
}

func (s *MetricsService) getPrevWaitSnapshot(id uuid.UUID) map[string][3]int64 {
	waitSnapshotMu.Lock()
	defer waitSnapshotMu.Unlock()
	return waitSnapshots[id]
}

func (s *MetricsService) storePrevWaitSnapshot(id uuid.UUID, snap map[string][3]int64) {
	waitSnapshotMu.Lock()
	defer waitSnapshotMu.Unlock()
	waitSnapshots[id] = snap
}

// StartSharedWaitStatsCollector starts the Tier-1 wait stats collection loop.
// Frequency is read from optima_collector_configs as
// "SQL Server Wait Stats Cumulative".
// One goroutine per monitored SQL Server instance.
func (s *MetricsService) StartSharedWaitStatsCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server Wait Stats Cumulative", 300*time.Second)
	slog.Info("[WaitStats] Starting shared wait stats collector (interval: %v)", "val", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Wait Stats Cumulative", 300*time.Second)
				if newInterval != interval && newInterval > 0 {
					interval = newInterval
					ticker.Reset(interval)
				}
				s.collectAndFanOutWaitStats(ctx)
			}
		}
	}()
}

func (s *MetricsService) collectAndFanOutWaitStats(ctx context.Context) {
	if !s.IsTimescaleConnected() {
		return
	}

	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}
		db, ok := s.MsRepo.GetConn(inst.Name)
		if !ok {
			continue
		}

		// Step A: Fetch cumulative snapshot from SQL Server DMV
		snapshots, err := s.waitDMVCollector.FetchCumulativeSnapshot(ctx, db)
		if err != nil {
			slog.Error("[WaitStats]", "target", inst.Name, "err", err)
			continue
		}

		// Step B: Write raw cumulative rows to TimescaleDB
		if err := s.tsLogger.WriteCumulativeSnapshot(ctx, inst.ServerID, snapshots); err != nil {
			slog.Error("[WaitStats]", "target", inst.Name, "err", err)
		}

		// Step C: Build current map for delta computation
		current := make(map[string][3]int64, len(snapshots))
		for _, snap := range snapshots {
			current[snap.WaitType] = [3]int64{snap.WaitTimeMs, snap.SignalWaitTimeMs, snap.WaitingTasksCount}
		}

		// Step D: Get previous snapshot from in-memory map (not from DB — faster)
		prev := s.getPrevWaitSnapshot(inst.ServerID)
		s.storePrevWaitSnapshot(inst.ServerID, current)

		if prev == nil {
			// First cycle — no deltas yet, skip writes
			continue
		}

		// Step E: Compute deltas (Go, zero allocation of SQL Server resources)
		deltas := dashboard.ComputeWaitDeltasV2(prev, current)

		// Step F: Write deltas to sqlserver_wait_stats_delta (new table, full granularity)
		deltaRows := make([]domain.WaitStatsDeltaRow, 0, len(deltas))
		for _, d := range deltas {
			if d.DeltaWaitMs == 0 && !d.RestartDetected {
				continue // skip zero-wait types to save storage
			}
			deltaRows = append(deltaRows, domain.WaitStatsDeltaRow{
				CaptureTimestamp:    time.Now().UTC(),
				WaitType:            d.WaitType,
				WaitCategory:        string(d.Category),
				DeltaWaitMs:         d.DeltaWaitMs,
				DeltaSignalWaitMs:   d.DeltaSignalWaitMs,
				DeltaResourceWaitMs: d.DeltaResourceWaitMs,
				DeltaWaitingTasks:   d.DeltaWaitingTasks,
				RestartDetected:     d.RestartDetected,
			})
		}
		if err := s.tsLogger.WriteWaitStatsDelta(ctx, inst.ServerID, deltaRows); err != nil {
			slog.Error("[WaitStats]", "target", inst.Name, "err", err)
		}

		// Step G: Also aggregate to the existing sqlserver_wait_stats table
		// (for Health Dashboard V2 category chart — no separate DMV hit needed)
		categoryMap := aggregateToCategories(deltas)
		for cat, totalMs := range categoryMap {
			if err := s.tsLogger.LogSQLServerWaitStatV2(ctx, inst.ServerID, cat, totalMs); err != nil {
				slog.Error(fmt.Sprintf("[WaitStats] %s: LogSQLServerWaitStatV2 %s error: %v", inst.Name, cat, err))
			}
		}
	}
}

// StartActiveWaitSessionsCollector starts the Tier-2 high-frequency collector.
// Collects detailed session info for currently waiting tasks.
func (s *MetricsService) StartActiveWaitSessionsCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server Active Wait Sessions", 30*time.Second)
	slog.Info("[WaitStats] Starting active sessions collector (interval: %v)", "val", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "SQL Server Active Wait Sessions", 30*time.Second)
				if newInterval != interval && newInterval > 0 {
					interval = newInterval
					ticker.Reset(interval)
				}
				s.collectActiveWaitSessions(ctx)
			}
		}
	}()
}

func (s *MetricsService) collectActiveWaitSessions(ctx context.Context) {
	if !s.IsTimescaleConnected() {
		return
	}
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}
		db, ok := s.MsRepo.GetConn(inst.Name)
		if !ok {
			continue
		}
		sessions, err := s.waitDMVCollector.FetchActiveWaitSessions(ctx, db)
		if err != nil {
			slog.Error("[WaitStats]", "target", inst.Name, "err", err)
			continue
		}
		if err := s.tsLogger.WriteActiveWaitSessions(ctx, inst.ServerID, sessions); err != nil {
			slog.Error("[WaitStats]", "target", inst.Name, "err", err)
		}
	}
}

// StartFileIOLatencyCollector starts the Tier-3 IO collector.
// Collects latency and throughput per database file.
func (s *MetricsService) StartFileIOLatencyCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "SQL Server File IO Latency", 60*time.Second)
	slog.Info("[WaitStats] Starting file IO latency collector (interval: %v)", "val", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "SQL Server File IO Latency", 60*time.Second)
				if newInterval != interval && newInterval > 0 {
					interval = newInterval
					ticker.Reset(interval)
				}
				s.collectFileIOLatency(ctx)
			}
		}
	}()
}

func (s *MetricsService) collectFileIOLatency(ctx context.Context) {
	if !s.IsTimescaleConnected() {
		return
	}
	for _, inst := range s.Config.Instances {
		if strings.ToLower(inst.Type) != "sqlserver" {
			continue
		}
		db, ok := s.MsRepo.GetConn(inst.Name)
		if !ok {
			continue
		}

		const q = `/* SQL_OPTIMA */
        SELECT
            ISNULL(DB_NAME(vfs.database_id),'Unknown'),
            mf.name as file_name,
            mf.type_desc as file_type,
            vfs.num_of_reads, vfs.num_of_bytes_read, vfs.io_stall_read_ms,
            vfs.num_of_writes, vfs.num_of_bytes_written, vfs.io_stall_write_ms
        FROM sys.dm_io_virtual_file_stats(NULL, NULL) AS vfs
        JOIN sys.master_files AS mf ON vfs.database_id = mf.database_id AND vfs.file_id = mf.file_id`
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			slog.Error("[WaitStats]", "target", inst.Name, "err", err)
			continue
		}

		current := make(map[string]ioSample)
		now := time.Now().UTC()
		for rows.Next() {
			var dbn, fn, ft string
			var r, rb, rs, w, wb, ws int64
			if err := rows.Scan(&dbn, &fn, &ft, &r, &rb, &rs, &w, &wb, &ws); err == nil {
				key := dbn + ":" + fn
				sample := ioSample{
					CapturedAt: now,
					Reads:      r,
					ReadBytes:  rb,
					ReadStall:  rs,
					Writes:     w,
					WriteBytes: wb,
					WriteStall: ws,
					FileType:   ft,
				}
				current[key] = sample
			}
		}
		rows.Close()

		prev := s.getPrevIOSnapshot(inst.ServerID)
		s.storePrevIOSnapshot(inst.ServerID, current)

		if prev == nil {
			continue
		}

		for key, currS := range current {
			prevS, ok := prev[key]
			if !ok {
				continue
			}
			parts := strings.Split(key, ":")
			dbName := parts[0]
			fileName := parts[1]
			fileType := currS.FileType

			elapsed := currS.CapturedAt.Sub(prevS.CapturedAt).Seconds()
			if elapsed <= 0 {
				continue
			}

			dReads := float64(currS.Reads - prevS.Reads)
			dWrites := float64(currS.Writes - prevS.Writes)
			dReadStall := float64(currS.ReadStall - prevS.ReadStall)
			dWriteStall := float64(currS.WriteStall - prevS.WriteStall)
			dReadBytes := float64(currS.ReadBytes - prevS.ReadBytes)
			dWriteBytes := float64(currS.WriteBytes - prevS.WriteBytes)

			if dReads < 0 {
				dReads = 0
			}
			if dWrites < 0 {
				dWrites = 0
			}

			readLat := 0.0
			if dReads > 0 {
				readLat = dReadStall / dReads
			}
			writeLat := 0.0
			if dWrites > 0 {
				writeLat = dWriteStall / dWrites
			}

			_ = s.tsLogger.LogSQLServerFileIOV2(ctx, inst.ServerID, dbName, fileName, fileType,
				readLat, writeLat, dReadBytes/elapsed, dWriteBytes/elapsed)
		}
	}
}

func (s *MetricsService) GetWaitStatsDashboardV2(
	ctx context.Context,
	serverID uuid.UUID,
	from, to time.Time,
) (models.WaitStatsDashboardResponse, error) {

	var res models.WaitStatsDashboardResponse
	res.LastUpdate = time.Now().UTC()

	// 1. Instance Name
	var instName string
	if s.Config != nil {
		for _, inst := range s.Config.Instances {
			if inst.ServerID == serverID {
				res.InstanceName = inst.Name
				instName = inst.Name
				break
			}
		}
	}

	if s.tsLogger == nil {
		return res, fmt.Errorf("TimescaleDB not connected")
	}

	// 2. KPIs
	kpis, err := s.tsLogger.GetWaitKPIsV2(ctx, serverID, from, to)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}
	res.KPIs = kpis

	// 3. Trends (Hourly/Raw)
	trends, err := s.tsLogger.GetWaitTrendsV2(ctx, serverID, from, to)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}
	res.WaitTrendsHourly = trends

	// 4. Daily Trends (for heatmap — expand window to up to 7 days)
	dailyFrom := to.AddDate(0, 0, -7)
	if from.Before(dailyFrom) {
		dailyFrom = from
	}
	dailyTrends, err := s.tsLogger.GetWaitTrendsV2(ctx, serverID, dailyFrom, to)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}
	res.WaitTrendsDaily = dailyTrends

	// 5. Top Wait Types
	top, err := s.tsLogger.GetTopWaitTypesV2(ctx, serverID, from, to, 10)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}
	res.TopWaitTypes = top

	// 6. CPU Pressure
	cpuPressure, err := s.tsLogger.GetCPUPressureTrend(ctx, serverID, from, to)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}
	res.CPUPressure = cpuPressure

	// 7. Database Impact
	dbImpact, err := s.tsLogger.GetDatabaseWaitImpact(ctx, serverID, from, to)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}
	res.DatabaseImpact = dbImpact

	// 8. Blocking Tree
	blockingTree, err := s.tsLogger.GetBlockingTree(ctx, serverID, from, to)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}
	res.BlockingTree = blockingTree

	// 9. Active Waits — most-recent snapshot in the selected time window.
	active, err := s.tsLogger.GetActiveWaitSessionsV2(ctx, serverID, from, to)
	if err != nil {
		slog.Error("[WaitStats]", "target", instName, "err", err)
	}

	res.ActiveWaits = active
	return res, nil
}

// aggregateToCategories sums delta_wait_ms per category from a list of deltas.
func aggregateToCategories(deltas []dashboard.WaitDeltaRowV2) map[string]float64 {
	m := make(map[string]float64, 8)
	for _, d := range deltas {
		m[string(d.Category)] += float64(d.DeltaWaitMs)
	}
	return m
}
