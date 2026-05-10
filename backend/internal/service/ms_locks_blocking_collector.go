// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: ms_locks_blocking_collector.go
// Purpose: Background collector for SQL Server blocking and deadlock telemetry.
//          Implements incident-aware delta deduplication to prevent massive duplicate
//          row growth in sqlserver_blocking_locks and sqlserver_blocking_snapshots.
// Metadata:
//   - Version: 1.2
//   - Package: service
//   - Collector: ms_blocking_snapshots, ms_deadlock_events
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// snapshotReinsertInterval controls how often an UNCHANGED blocking session is
// re-logged to capture wait_duration_ms progression. 60 s means at most 1 row/min
// per session instead of 1 row every 10 s — a 6× reduction even for existing sessions.
const snapshotReinsertInterval = 60 * time.Second

// maxDeadlockCacheSize caps the in-memory deduplication set for deadlock events
// so it does not grow unboundedly over a very long uptime.
const maxDeadlockCacheSize = 1000

// msInstanceBlockingState holds per-instance deduplication state across polls.
// All fields are protected by the embedded mutex.
type msInstanceBlockingState struct {
	mu sync.Mutex

	// insertedLockFPs tracks lock "fingerprints" already written for the current
	// blocking incident.
	// Key: "<sessionID>|<resourceType>|<requestMode>|<requestStatus>|<resourceDesc>"
	// Cleared when no blocking is detected (incident over) so fresh data is captured
	// for the next incident.
	insertedLockFPs map[string]struct{}

	// snapLastInserted tracks when each session state was last persisted.
	// Key: "<sessionID>|<blockingSessionID>|<waitType>"
	// Value: time of last INSERT
	// A row is inserted when: (a) the key is brand-new, or (b) the key exists but
	// snapshotReinsertInterval has elapsed (to capture wait_duration growth over time).
	snapLastInserted map[string]time.Time

	// prevActiveSIDs is the set of session IDs (blockers + blocked) seen in the
	// most-recent poll that had blocking. Used to detect incident end.
	prevActiveSIDs map[int]struct{}

	// insertedDeadlocks prevents re-inserting events that are re-read from the XE
	// file on every poll (the query always reads the last 24 h from the file).
	// Key: "<ts_rounded_to_sec>|<victimSessionID>"
	insertedDeadlocks map[string]struct{}
}

func newMsInstanceBlockingState() *msInstanceBlockingState {
	return &msInstanceBlockingState{
		insertedLockFPs:   make(map[string]struct{}),
		snapLastInserted:  make(map[string]time.Time),
		prevActiveSIDs:    make(map[int]struct{}),
		insertedDeadlocks: make(map[string]struct{}),
	}
}

// incidentActive reports whether there were any blocking sessions in the last poll.
func (st *msInstanceBlockingState) incidentActive() bool {
	return len(st.prevActiveSIDs) > 0
}

// resetIncident clears lock and snapshot caches when blocking resolves, so the
// next incident starts with a clean slate.
func (st *msInstanceBlockingState) resetIncident() {
	st.insertedLockFPs = make(map[string]struct{})
	st.snapLastInserted = make(map[string]time.Time)
	st.prevActiveSIDs = make(map[int]struct{})
}

// lockFP builds the deduplication fingerprint for a blocking lock row.
func lockFP(l models.SQLServerBlockingLock) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s",
		l.SessionID, l.ResourceType, l.RequestMode, l.RequestStatus, l.ResourceDescription)
}

// snapFP builds the deduplication fingerprint for a blocking snapshot row.
func snapFP(s models.SQLServerBlockingSnapshot) string {
	return fmt.Sprintf("%d|%d|%s", s.SessionID, s.BlockingSessionID, s.WaitType)
}

// deadlockFP builds the deduplication fingerprint for a deadlock event.
func deadlockFP(e models.SQLServerDeadlockEvent) string {
	return fmt.Sprintf("%s|%d",
		e.Timestamp.UTC().Round(time.Second).Format(time.RFC3339),
		e.VictimSessionID)
}

// ----------------------------------------------------------------------------

func (s *MetricsService) StartMsLocksBlockingCollector(ctx context.Context) {
	if s.tsLogger == nil {
		log.Printf("[MS LocksBlocking] Timescale not connected; collector disabled")
		return
	}

	for _, inst := range s.Config.Instances {
		if inst.Type != "sqlserver" {
			continue
		}
		instanceName := inst.Name

		go func(name string) {
			// Default frequency
			freq := 10 * time.Second
			if s.ConfigRepo != nil {
				if cfg, err := s.ConfigRepo.GetByName(ctx, "sqlserver_blocking"); err == nil && cfg != nil && cfg.FrequencySeconds > 0 {
					freq = time.Duration(cfg.FrequencySeconds) * time.Second
				}
			}

			ticker := time.NewTicker(freq)
			defer ticker.Stop()

			log.Printf("[MS LocksBlocking] Starting collector for %s every %v", name, freq)

			// Check XE session on startup
			db, ok := s.MsRepo.GetConn(name)
			if ok {
				if err := s.MsRepo.EnsureDeadlockXESession(db); err != nil {
					log.Printf("[MS LocksBlocking] Failed to ensure Deadlock XE session for %s: %v", name, err)
				}
			}

			// Per-instance deduplication state — lives for the lifetime of this goroutine.
			state := newMsInstanceBlockingState()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Check for frequency changes
					if s.ConfigRepo != nil {
						if cfg, err := s.ConfigRepo.GetByName(ctx, "sqlserver_blocking"); err == nil && cfg != nil && cfg.FrequencySeconds > 0 {
							newFreq := time.Duration(cfg.FrequencySeconds) * time.Second
							if newFreq != freq {
								freq = newFreq
								ticker.Reset(freq)
								log.Printf("[MS LocksBlocking] Updated frequency for %s to %v", name, freq)
							}
						}
					}
					s.collectAndLogMsBlocking(ctx, name, state)
				}
			}
		}(instanceName)
	}
}

func (s *MetricsService) collectAndLogMsBlocking(ctx context.Context, instanceName string, state *msInstanceBlockingState) {
	db, ok := s.MsRepo.GetConn(instanceName)
	if !ok {
		return
	}

	// ------------------------------------------------------------------ //
	// 1. Fetch Blocking Snapshots
	// ------------------------------------------------------------------ //
	snapshots, err := s.MsRepo.FetchBlockingSnapshots(db)
	if err != nil {
		log.Printf("[MS LocksBlocking] FetchBlockingSnapshots error for %s: %v", instanceName, err)
		snapshots = nil
	}

	state.mu.Lock()

	if len(snapshots) == 0 {
		// No active blocking. If there was an incident running, close it out and
		// reset the deduplication caches so the next incident starts fresh.
		if state.incidentActive() {
			log.Printf("[MS LocksBlocking] Incident resolved for %s — resetting dedup state", instanceName)
			state.resetIncident()
		}
		state.mu.Unlock()

	} else {
		// Build the current session-ID set (both blockers and blocked).
		currentSIDs := make(map[int]struct{}, len(snapshots))
		for _, snap := range snapshots {
			currentSIDs[snap.SessionID] = struct{}{}
		}

		// --- Delta snapshots: only write rows that are new or stale ---
		now := time.Now()
		var deltaSnapshots []models.SQLServerBlockingSnapshot
		for _, snap := range snapshots {
			fp := snapFP(snap)
			lastInsert, seen := state.snapLastInserted[fp]
			if !seen || now.Sub(lastInsert) >= snapshotReinsertInterval {
				deltaSnapshots = append(deltaSnapshots, snap)
				state.snapLastInserted[fp] = now
			}
		}

		// Purge fingerprints for sessions that have left the blocking chain so the
		// map doesn't retain stale entries across incidents.
		for fp := range state.snapLastInserted {
			var sid, bsid int
			var wt string
			if _, err := fmt.Sscanf(fp, "%d|%d|%s", &sid, &bsid, &wt); err != nil {
				continue
			}
			if _, active := currentSIDs[sid]; !active {
				delete(state.snapLastInserted, fp)
			}
		}

		state.prevActiveSIDs = currentSIDs
		state.mu.Unlock()

		// Upsert dimension data and persist delta snapshots.
		if len(deltaSnapshots) > 0 {
			log.Printf("[MS LocksBlocking] Writing %d/%d snapshot rows (delta) for %s",
				len(deltaSnapshots), len(snapshots), instanceName)

			for i := range deltaSnapshots {
				snap := &deltaSnapshots[i]
				if snap.QueryText != "" {
					if snap.SqlHash == "" {
						snap.SqlHash = fmt.Sprintf("%x", sha256.Sum256([]byte(snap.QueryText)))
					}
					if err := s.tsLogger.UpsertSQLServerTextDim(ctx, snap.SqlHash, snap.QueryText); err != nil {
						log.Printf("[MS LocksBlocking] UpsertSQLServerTextDim error: %v", err)
					}
				}
				if snap.QueryPlan != "" {
					if snap.PlanHash == "" {
						snap.PlanHash = fmt.Sprintf("%x", sha256.Sum256([]byte(snap.QueryPlan)))
					}
					if err := s.tsLogger.UpsertSQLServerQueryPlanDim(ctx, snap.PlanHash, snap.QueryPlan); err != nil {
						log.Printf("[MS LocksBlocking] UpsertSQLServerQueryPlanDim error: %v", err)
					}
				}
			}

			if err := s.tsLogger.LogSQLServerBlockingSnapshots(ctx, instanceName, deltaSnapshots); err != nil {
				log.Printf("[MS LocksBlocking] LogSQLServerBlockingSnapshots error for %s: %v", instanceName, err)
			}
		} else {
			log.Printf("[MS LocksBlocking] Skipping snapshot insert for %s — no delta (%d sessions already current)",
				instanceName, len(snapshots))
		}

		// ------------------------------------------------------------------ //
		// 2. Fetch Detailed Locks — only write NEW fingerprints this incident.
		// ------------------------------------------------------------------ //
		locks, lockErr := s.MsRepo.FetchBlockingLocks(db)
		if lockErr != nil {
			log.Printf("[MS LocksBlocking] FetchBlockingLocks error for %s: %v", instanceName, lockErr)
			locks = nil
		}

		if len(locks) > 0 {
			state.mu.Lock()
			var deltaLocks []models.SQLServerBlockingLock
			for _, l := range locks {
				fp := lockFP(l)
				if _, seen := state.insertedLockFPs[fp]; !seen {
					deltaLocks = append(deltaLocks, l)
					state.insertedLockFPs[fp] = struct{}{}
				}
			}
			state.mu.Unlock()

			if len(deltaLocks) > 0 {
				log.Printf("[MS LocksBlocking] Writing %d/%d lock rows (new this incident) for %s",
					len(deltaLocks), len(locks), instanceName)
				if err := s.tsLogger.LogSQLServerBlockingLocks(ctx, instanceName, deltaLocks); err != nil {
					log.Printf("[MS LocksBlocking] LogSQLServerBlockingLocks error for %s: %v", instanceName, err)
				}
			} else {
				log.Printf("[MS LocksBlocking] Skipping lock insert for %s — all %d locks already recorded this incident",
					instanceName, len(locks))
			}
		}
	}

	// ------------------------------------------------------------------ //
	// 3. Fetch Deadlocks — deduplicate by (timestamp, victimSessionID).
	//    The XE query always reads the last 24 h so without deduplication
	//    the same event is inserted on every poll that covers its window.
	// ------------------------------------------------------------------ //
	deadlocks, err := s.MsRepo.FetchDeadlockEvents(db)
	if err != nil {
		log.Printf("[MS LocksBlocking] FetchDeadlockEvents error for %s: %v", instanceName, err)
		return
	}

	if len(deadlocks) > 0 {
		state.mu.Lock()

		// Evict the cache if it has grown too large. Deadlocks are rare so this
		// almost never happens, but we guard against it for long-running servers.
		if len(state.insertedDeadlocks) > maxDeadlockCacheSize {
			log.Printf("[MS LocksBlocking] Deadlock dedup cache exceeded %d entries for %s — evicting",
				maxDeadlockCacheSize, instanceName)
			state.insertedDeadlocks = make(map[string]struct{})
		}

		var deltaDeadlocks []models.SQLServerDeadlockEvent
		for _, e := range deadlocks {
			fp := deadlockFP(e)
			if _, seen := state.insertedDeadlocks[fp]; !seen {
				deltaDeadlocks = append(deltaDeadlocks, e)
				state.insertedDeadlocks[fp] = struct{}{}
			}
		}
		state.mu.Unlock()

		if len(deltaDeadlocks) > 0 {
			log.Printf("[MS LocksBlocking] Writing %d new deadlock events for %s", len(deltaDeadlocks), instanceName)
			if err := s.tsLogger.LogSQLServerDeadlockEvents(ctx, instanceName, deltaDeadlocks); err != nil {
				log.Printf("[MS LocksBlocking] LogSQLServerDeadlockEvents error for %s: %v", instanceName, err)
			}
		}
	}
}
