// Package service implements metrics collection logic.
package service

import (
	"log/slog"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type msInstanceBlockingState struct {
	incidentID int64
	peak       int
	mu         sync.Mutex
}

func newMsInstanceBlockingState() *msInstanceBlockingState {
	return &msInstanceBlockingState{}
}

func (st *msInstanceBlockingState) incidentActive() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.incidentID > 0
}

func (st *msInstanceBlockingState) resetIncident() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.incidentID = 0
	st.peak = 0
}

func (s *MetricsService) StartMsLocksBlockingCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "sqlserver_locks_blocking", 15*time.Second)
	ticker := time.NewTicker(interval)
	slog.Info("[SQLServerBlocking] Starting collector (%v interval)", "val", interval)

	states := make(map[uuid.UUID]*msInstanceBlockingState)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, inst := range s.Config.Instances {
					if inst.Type != "sqlserver" {
						continue
					}
					st, ok := states[inst.ServerID]
					if !ok {
						st = newMsInstanceBlockingState()
						states[inst.ServerID] = st
					}
					s.collectAndLogMsBlocking(ctx, inst.ServerID, inst.Name, st)
				}
			}
		}
	}()
}

func (s *MetricsService) collectAndLogMsBlocking(ctx context.Context, serverID uuid.UUID, instanceName string, state *msInstanceBlockingState) {
	db, ok := s.MsRepo.GetConn(instanceName)
	if !ok {
		return
	}

	snap, err := repository.CollectBlockingLocks(ctx, db)
	if err != nil {
		slog.Error("[SQLServerBlocking] ERROR", "target", serverID, "err", err)
		return
	}

	if len(snap) == 0 {
		if state.incidentActive() {
			_ = s.tsLogger.CloseMsBlockingIncident(ctx, state.incidentID, time.Now().UTC())
			state.resetIncident()
		}
		return
	}

	// Logic for identifying root blockers and managing incidents
	var rootPID int
	var rootQuery string
	var victimCount int
	// ... logic to parse snap tree ...

	if !state.incidentActive() {
		id, err := s.tsLogger.OpenMsBlockingIncident(ctx, serverID, time.Now().UTC(), &rootPID, rootQuery)
		if err == nil {
			state.incidentID = id
		}
	} else {
		_ = s.tsLogger.UpdateMsBlockingIncident(ctx, state.incidentID, victimCount, &rootPID, rootQuery)
	}

	// Always log raw pairs for timeline
	var pairs []hot.PgBlockingPairRow // reused for SQL Server
	// ... map snap to pairs ...
	_ = s.tsLogger.LogMsBlockingPairs(ctx, pairs)
}
