// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Collector engine for coordinating data collection from multiple database sources.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package collectors

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
	"github.com/rsharma155/sql_optima/internal/repository"
)

const (
	LiveInterval       = 15 * time.Second
	HistoricalInterval = 60 * time.Second
)

type CollectorResult struct {
	CPU              *models.CPUTick
	Memory           *models.MemoryStats
	WaitStats        []models.WaitStat
	FileStats        []models.FileIOStat
	TempDBStats      *models.TempDBStats
	ActiveQueries    []models.ActiveQuery
	LongRunning      []models.LongRunningQuery
	Blocking         []models.BlockingNode
	SessionSnapshots []models.SQLServerSessionSnapshot
	Errors           []error
}

type SQLSERVERCollector struct {
	conns      map[string]*sql.DB
	serverIDs  map[string]uuid.UUID
	mu         sync.RWMutex
	result     CollectorResult
	liveTicker *time.Ticker
	histTicker *time.Ticker
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewSQLSERVERCollector(conns map[string]*sql.DB, serverIDs map[string]uuid.UUID) *SQLSERVERCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &SQLSERVERCollector{
		conns:     conns,
		serverIDs: serverIDs,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (c *SQLSERVERCollector) Start() {
	c.liveTicker = time.NewTicker(LiveInterval)
	c.histTicker = time.NewTicker(HistoricalInterval)

	slog.Info("[Collector] Split-Speed Background Daemon starting...")
	slog.Info("[Collector]   - Live Diagnostics ticker: every", "val", LiveInterval)
	slog.Info("[Collector]   - Historical Storage ticker: every", "val", HistoricalInterval)
	slog.Info("[Collector]   - Live collectors: queries_active.go, blocking_locks.go (every 15s)")
	slog.Info("[Collector]   - Historical collectors: cpu_memory.go, waits.go, storage_io.go (every 60s)")

	go func() {
		for {
			select {
			case <-c.ctx.Done():
				slog.Info("[Collector] Background daemon shutting dow")
				return

			case <-c.liveTicker.C:
				c.runLiveCollectors()

			case <-c.histTicker.C:
				c.runHistoricalCollectors()
			}
		}
	}()
}

func (c *SQLSERVERCollector) Stop() {
	c.cancel()
	c.liveTicker.Stop()
	c.histTicker.Stop()
	c.wg.Wait()
	slog.Info("[Collector] Stopped all collectors")
}

func (c *SQLSERVERCollector) GetResult() CollectorResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.result
}

func (c *SQLSERVERCollector) runLiveCollectors() {
	var wg sync.WaitGroup
	errors := []error{}

	c.mu.Lock()
	conns := c.conns
	c.mu.Unlock()

	for name, db := range conns {
		serverID := c.serverIDs[name]
		wg.Add(1)
		go func(instanceName string, serverID uuid.UUID, db *sql.DB) {
			defer wg.Done()

			queries, err := repository.CollectActiveQueries(c.ctx, db)
			if err != nil {
				slog.Error("[Collector] ERROR CollectActiveQueries", "target", instanceName, "err", err)
				errors = append(errors, fmt.Errorf("active queries: %w", err))
			} else {
				c.mu.Lock()
				c.result.ActiveQueries = queries
				c.mu.Unlock()
			}

			blocking, err := repository.CollectBlockingLocks(c.ctx, db)
			if err != nil {
				slog.Error("[Collector] ERROR CollectBlockingLocks", "target", instanceName, "err", err)
				errors = append(errors, fmt.Errorf("blocking locks: %w", err))
			} else {
				c.mu.Lock()
				c.result.Blocking = blocking
				c.mu.Unlock()
			}

			snapshots, err := repository.CollectSessionSnapshot(c.ctx, db)
			if err != nil {
				slog.Error("[Collector] ERROR CollectSessionSnapshot", "target", instanceName, "err", err)
				errors = append(errors, fmt.Errorf("session snapshot: %w", err))
			} else {
				c.mu.Lock()
				// Add serverID ID to snapshots
				for i := range snapshots {
					snapshots[i].ServerID = serverID
				}
				c.result.SessionSnapshots = append(c.result.SessionSnapshots, snapshots...)
				c.mu.Unlock()
			}
		}(name, serverID, db)
	}

	wg.Wait()

	c.mu.Lock()
	c.result.Errors = errors
	c.mu.Unlock()

	slog.Error(fmt.Sprintf("[Collector] Live tick complete - ActiveQueries: %d, Blocking: %d, SessionSnapshots: %d, Errors: %d", len(c.result.ActiveQueries), len(c.result.Blocking), len(c.result.SessionSnapshots), len(errors)))
}

func (c *SQLSERVERCollector) runHistoricalCollectors() {
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := []error{}

	c.mu.Lock()
	conns := c.conns
	c.mu.Unlock()

	for name, db := range conns {
		serverID := c.serverIDs[name]
		wg.Add(1)
		go func(instanceName string, serverID uuid.UUID, db *sql.DB) {
			defer wg.Done()

			cpu, mem, err := repository.CollectCPUMemory(c.ctx, db)
			if err != nil {
				slog.Error("[Collector] ERROR CollectCPUMemory", "target", instanceName, "err", err)
				mu.Lock()
				errors = append(errors, fmt.Errorf("cpu/memory: %w", err))
				mu.Unlock()
			} else {
				c.mu.Lock()
				c.result.CPU = cpu
				c.result.Memory = mem
				c.mu.Unlock()
			}

			waits, err := repository.CollectWaitStats(c.ctx, db)
			if err != nil {
				slog.Error("[Collector] ERROR CollectWaitStats", "target", instanceName, "err", err)
				mu.Lock()
				errors = append(errors, fmt.Errorf("wait stats: %w", err))
				mu.Unlock()
			} else {
				c.mu.Lock()
				c.result.WaitStats = waits
				c.mu.Unlock()
			}

			storage, tempdb, err := repository.CollectStorageIO(c.ctx, db)
			if err != nil {
				slog.Error("[Collector] ERROR CollectStorageIO", "target", instanceName, "err", err)
				mu.Lock()
				errors = append(errors, fmt.Errorf("storage I/O: %w", err))
				mu.Unlock()
			} else {
				c.mu.Lock()
				c.result.FileStats = storage
				c.result.TempDBStats = tempdb
				c.mu.Unlock()
			}

			longRunning, err := repository.CollectLongRunningQueries(c.ctx, db)
			if err != nil {
				slog.Error("[Collector] ERROR CollectLongRunningQueries", "target", instanceName, "err", err)
				mu.Lock()
				errors = append(errors, fmt.Errorf("long running: %w", err))
				mu.Unlock()
			} else {
				c.mu.Lock()
				c.result.LongRunning = longRunning
				c.mu.Unlock()
			}
		}(name, serverID, db)
	}

	wg.Wait()

	c.mu.Lock()
	c.result.Errors = errors
	c.mu.Unlock()

	slog.Error(fmt.Sprintf("[Collector] Historical tick complete - CPU: %v, Memory: %v, Waits: %d, Storage: %d, TempDB: %v, LongRunning: %d, Errors: %d", c.result.CPU != nil, c.result.Memory != nil, len(c.result.WaitStats), len(c.result.FileStats), c.result.TempDBStats != nil, len(c.result.LongRunning), len(errors)))
}
