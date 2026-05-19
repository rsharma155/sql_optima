// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Background collector for PostgreSQL locks and blocking trees.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type pgInstanceBlockingState struct {
	incidentID int64
	peak       int
	mu         sync.Mutex
}

func (s *MetricsService) StartPgLocksBlockingCollector(ctx context.Context) {
	interval := s.GetCollectorInterval(ctx, "pg_locks_blocking", 15*time.Second)
	ticker := time.NewTicker(interval)
	slog.Info("[PGLocksBlocking] Starting collector (%v interval)", "val", interval)

	states := make(map[uuid.UUID]*pgInstanceBlockingState)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, inst := range s.Config.Instances {
					if strings.ToLower(inst.Type) != "postgres" {
						continue
					}
					st, ok := states[inst.ServerID]
					if !ok {
						st = &pgInstanceBlockingState{}
						states[inst.ServerID] = st
					}
					s.runPgLocksBlockingForInstance(ctx, inst.ServerID, st)
				}
			}
		}
	}()
}

func (s *MetricsService) runPgLocksBlockingForInstance(ctx context.Context, serverID uuid.UUID, st *pgInstanceBlockingState) {
	instanceName := ""
	for _, inst := range s.Config.Instances {
		if inst.ServerID == serverID {
			instanceName = inst.Name
			break
		}
	}
	if instanceName == "" {
		return
	}

	db, ok := s.PgRepo.GetConn(instanceName)
	if !ok {
		return
	}

	now := time.Now().UTC()

	// 1. Session Snapshot
	if err := s.collectPgSessionsSnapshot(ctx, db, serverID, now); err != nil {
		slog.Error("[PGLocksBlocking] Sessions error", "target", serverID, "err", err)
	}

	// 2. Locks Snapshot
	if err := s.collectPgLocksSnapshot(ctx, db, serverID, now); err != nil {
		slog.Error("[PGLocksBlocking] Locks error", "target", serverID, "err", err)
	}

	// 3. Blocking Pairs & Incident Management
	pairs, victims, rootPID, rootQuery, err := s.collectPgBlockingPairs(ctx, db, serverID, now)
	if err != nil {
		slog.Error("[PGLocksBlocking] BlockingPairs error", "target", serverID, "err", err)
	}

	if len(pairs) > 0 {
		if err := s.tsLogger.LogPgBlockingPairs(ctx, pairs); err != nil {
			slog.Error("[PGLocksBlocking] LogPgBlockingPairs error", "target", serverID, "err", err)
		}
	}

	if s.tsLogger == nil {
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if len(pairs) > 0 {
		if st.incidentID == 0 {
			id, err := s.tsLogger.OpenPgBlockingIncident(ctx, serverID, now, rootPID, rootQuery)
			if err != nil {
				slog.Error("[PGLocksBlocking] OpenIncident error", "target", serverID, "err", err)
			} else {
				st.incidentID = id
				st.peak = victims
			}
		} else {
			if victims > st.peak {
				st.peak = victims
			}
			if err := s.tsLogger.UpdatePgBlockingIncident(ctx, st.incidentID, st.peak, rootPID, rootQuery); err != nil {
				slog.Error("[PGLocksBlocking] UpdateIncident error", "target", serverID, "err", err)
			}
		}
	} else if st.incidentID != 0 {
		if err := s.tsLogger.ClosePgBlockingIncident(ctx, st.incidentID, now); err != nil {
			slog.Error("[PGLocksBlocking] CloseIncident error", "target", serverID, "err", err)
		}
		st.incidentID = 0
		st.peak = 0
	}
}

func (s *MetricsService) collectPgSessionsSnapshot(ctx context.Context, db *sql.DB, serverID uuid.UUID, now time.Time) error {
	if s.PgObsCollector != nil {
		return s.PgObsCollector.CollectSessionActivity(ctx, serverID, db)
	}
	return nil
}

func (s *MetricsService) collectPgLocksSnapshot(ctx context.Context, db *sql.DB, serverID uuid.UUID, now time.Time) error {
	if s.PgTimescaleColl != nil {
		instanceName := s.getServerName(serverID)
		if instanceName == "" {
			return fmt.Errorf("instance not found for %s", serverID)
		}
		// Directly call Repo for detailed locks and log to Timescale
		locks, err := s.PgRepo.FetchDetailedLocks(ctx, instanceName, db)
		if err != nil {
			return err
		}
		
		var hotLocks []hot.PgLockSnapshotRow
		for _, l := range locks {
			hotLocks = append(hotLocks, hot.PgLockSnapshotRow{
				CollectedAt:    now,
				ServerID:       serverID,
				PID:            l.PID,
				LockType:       l.LockType,
				Mode:           l.Mode,
				Granted:        l.Granted,
				RelationOID:    l.RelationOID,
				RelationName:   l.RelationName,
				TransactionID:  l.TransactionID,
				WaitingSeconds: l.WaitDurationMs / 1000.0,
			})
		}
		return s.tsLogger.LogPgLockSnapshot(ctx, hotLocks)
	}
	return nil
}

func (s *MetricsService) collectPgBlockingPairs(ctx context.Context, db *sql.DB, serverID uuid.UUID, now time.Time) ([]hot.PgBlockingPairRow, int, *int, string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT w.pid AS blocked_pid, bl_pid AS blocking_pid
		FROM pg_stat_activity w, unnest(pg_blocking_pids(w.pid)) AS bl_pid
		WHERE cardinality(pg_blocking_pids(w.pid)) > 0`)
	if err != nil {
		return nil, 0, nil, "", err
	}
	defer rows.Close()

	var pairs []hot.PgBlockingPairRow
	blockedSet := make(map[int]bool)
	blockerSet := make(map[int]bool)
	for rows.Next() {
		var blockedPID, blockingPID int
		if err := rows.Scan(&blockedPID, &blockingPID); err == nil {
			pairs = append(pairs, hot.PgBlockingPairRow{
				CollectedAt: now,
				ServerID:    serverID,
				BlockedPID:  blockedPID,
				BlockingPID: blockingPID,
			})
			blockedSet[blockedPID] = true
			blockerSet[blockingPID] = true
		}
	}

	// Root blocker: a blocker PID that is not itself blocked
	var rootPID *int
	var rootQuery string
	for pid := range blockerSet {
		if !blockedSet[pid] {
			p := pid
			rootPID = &p
			_ = db.QueryRowContext(ctx, `SELECT COALESCE(LEFT(query, 200), '') FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&rootQuery)
			break
		}
	}

	return pairs, len(blockedSet), rootPID, rootQuery, nil
}
