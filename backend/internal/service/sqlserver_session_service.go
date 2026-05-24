// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background service that collects SQL Server active sessions every 30 s
// and writes them to the sqlserver_active_sessions hypertable. Also collects
// per-session memory grants when grants are pending.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sqlserver "github.com/rsharma155/sql_optima/internal/collectors/infrastructure/sqlserver"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// sessionDeltaState tracks the last snapshot hash per server to avoid writing
// identical session data every cycle.
type sessionDeltaState struct {
	mu       sync.Mutex
	lastHash string
	lastTime time.Time
}

func (st *sessionDeltaState) isChanged(rows []sqlserver.ActiveSessionRow) bool {
	h := hashSessionRows(rows)
	st.mu.Lock()
	defer st.mu.Unlock()
	changed := h != st.lastHash
	if changed {
		st.lastHash = h
		st.lastTime = time.Now().UTC()
	}
	return changed
}

func hashSessionRows(rows []sqlserver.ActiveSessionRow) string {
	h := sha256.New()
	for _, r := range rows {
		fmt.Fprintf(h, "%d|%d|%s|", r.SessionID, r.BlockingSessionID, r.RequestStatus)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// StartSessionCollector launches a background goroutine that collects active
// sessions every 30 seconds and writes them to sqlserver_active_sessions.
func (s *MetricsService) StartSessionCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "sqlserver_sessions", 30*time.Second)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	slog.Info("[SessionCollector] Starting", "interval", interval)

	collector := &sqlserver.SessionCollector{}
	states := make(map[uuid.UUID]*sessionDeltaState)
	var mu sync.Mutex

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				newInterval := s.GetCollectorInterval(ctx, "sqlserver_sessions", 30*time.Second)
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
						s.collectSessionsForInstance(ctx, collector, states, &mu, instance.ServerID, instance.Name)
					})
				}
			}
		}
	}()
}

func (s *MetricsService) collectSessionsForInstance(
	ctx context.Context,
	collector *sqlserver.SessionCollector,
	states map[uuid.UUID]*sessionDeltaState,
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
	if s.tsLogger == nil {
		return
	}

	rows, err := collector.Fetch(ctx, db)
	if err != nil {
		slog.Error("[SessionCollector] fetch failed", "instance", instanceName, "err", err)
		return
	}

	mu.Lock()
	st, exists := states[serverID]
	if !exists {
		st = &sessionDeltaState{}
		states[serverID] = st
	}
	mu.Unlock()

	// Skip write if sessions unchanged since last cycle.
	if !st.isChanged(rows) {
		return
	}

	now := time.Now().UTC()
	writeRows := make([]hot.ActiveSessionWriteRow, 0, len(rows))
	for _, r := range rows {
		writeRows = append(writeRows, hot.ActiveSessionWriteRow{
			SessionID:            r.SessionID,
			LoginName:            r.LoginName,
			HostName:             r.HostName,
			ProgramName:          r.ProgramName,
			DatabaseName:         r.DatabaseName,
			RequestStatus:        r.RequestStatus,
			WaitType:             r.WaitType,
			WaitTimeMs:           r.WaitTimeMs,
			BlockingSessionID:    r.BlockingSessionID,
			CPUTimeMs:            r.CPUTimeMs,
			TotalElapsedMs:       r.TotalElapsedMs,
			LogicalReads:         r.LogicalReads,
			Reads:                r.Reads,
			Writes:               r.Writes,
			GrantedQueryMemoryKB: r.GrantedQueryMemoryKB,
			DOP:                  r.DOP,
			QueryHash:            r.QueryHash,
			QueryText:            r.QueryText,
		})
	}

	if err := s.tsLogger.LogSqlServerActiveSessions(ctx, serverID, now, writeRows); err != nil {
		slog.Error("[SessionCollector] write failed", "instance", instanceName, "err", err)
	}
}
