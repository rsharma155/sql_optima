// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background service that collects sys.dm_os_performance_counters once
//          per cycle (30 s default) and writes the unified snapshot to TimescaleDB.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/collectors/infrastructure/sqlserver"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// pcCollectorState holds the previous snapshot per counter (name+instance) for
// a single monitored server. Used to compute delta-based rates.
type pcCollectorState struct {
	mu       sync.Mutex
	prev     map[string]int64 // key: "counterName|instanceName"
	prevTime time.Time
}

func newPCCollectorState() *pcCollectorState {
	return &pcCollectorState{prev: make(map[string]int64)}
}

// computeRows converts raw DMV rows into write rows, computing rates for
// cumulative counters (cntr_type 272696576) using the stored previous snapshot.
func (st *pcCollectorState) computeRows(rows []sqlserver.PerfCounterRow, now time.Time) []hot.PerfCounterWriteRow {
	st.mu.Lock()
	defer st.mu.Unlock()

	intervalSecs := 0.0
	if !st.prevTime.IsZero() {
		intervalSecs = now.Sub(st.prevTime).Seconds()
	}

	out := make([]hot.PerfCounterWriteRow, 0, len(rows))
	for _, r := range rows {
		key := r.CounterName + "|" + r.InstanceName
		rate := 0.0
		const cumulativeType = 272696576
		if r.CntrType == cumulativeType {
			if prev, ok := st.prev[key]; ok {
				rate = sqlserver.ComputeRatePerSec(r.CntrValue, prev, intervalSecs)
			}
		} else {
			// Point-in-time counter: use raw value as rate
			rate = float64(r.CntrValue)
		}
		st.prev[key] = r.CntrValue
		out = append(out, hot.PerfCounterWriteRow{
			CounterName:  r.CounterName,
			InstanceName: r.InstanceName,
			CntrValue:    r.CntrValue,
			CntrType:     r.CntrType,
			RatePerSec:   rate,
		})
	}
	st.prevTime = now
	return out
}

// StartPerfCountersCollector launches a background goroutine that runs the
// unified sys.dm_os_performance_counters collector every 30 seconds.
func (s *MetricsService) StartPerfCountersCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "sqlserver_perf_counters", 30*time.Second)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	slog.Info("[PerfCounters] Starting collector", "interval", interval)

	collector := &sqlserver.PerfCountersCollector{}
	states := make(map[uuid.UUID]*pcCollectorState)
	var mu sync.Mutex

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "sqlserver_perf_counters", 30*time.Second)
				if newInterval != interval && newInterval > 0 {
					interval = newInterval
					ticker.Reset(interval)
				}

				for _, inst := range s.Config.Instances {
					if strings.ToLower(inst.Type) != "sqlserver" {
						continue
					}
					instance := inst
					s.EnqueueCollection(instance.ServerID, func() {
						s.collectPerfCountersForInstance(ctx, collector, states, &mu, instance.ServerID, instance.Name)
					})
				}
			}
		}
	}()
}

func (s *MetricsService) collectPerfCountersForInstance(
	ctx context.Context,
	collector *sqlserver.PerfCountersCollector,
	states map[uuid.UUID]*pcCollectorState,
	mu *sync.Mutex,
	serverID uuid.UUID,
	instanceName string,
) {
	if s.MsRepo == nil {
		return
	}
	db, ok := s.MsRepo.GetConn(instanceName)
	if !ok || db == nil {
		return
	}

	rows, err := collector.Fetch(ctx, db)
	if err != nil {
		slog.Error("[PerfCounters] fetch failed", "instance", instanceName, "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	mu.Lock()
	st, exists := states[serverID]
	if !exists {
		st = newPCCollectorState()
		states[serverID] = st
	}
	mu.Unlock()

	now := time.Now().UTC()
	writeRows := st.computeRows(rows, now)
	if s.tsLogger == nil {
		return
	}
	if err := s.tsLogger.LogSqlServerPerfCountersV2(ctx, serverID, now, writeRows); err != nil {
		slog.Error("[PerfCounters] write failed", "instance", instanceName, "err", err)
	}
}
