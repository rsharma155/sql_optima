// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose : 30-second HA replica and database state collector.
//           Captures AG replica health, commit lag, and per-DB sync state.
//           Also implements failover detection by comparing primary names.
//
// Author  : Ravi Sharma <ravisharma155@gmail.com>
// Created : 2026-05-14
// License : MIT
package collectors

import (
	"fmt"
	"log/slog"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// HAReplicaCollector handles high-frequency AG health snapshots.
type HAReplicaCollector struct {
	querier       *SourceQuerier
	tsLogger      *hot.TimescaleLogger
	prevPrimaries map[uuid.UUID]string
	mu            sync.Mutex
}

// NewHAReplicaCollector constructs the collector.
func NewHAReplicaCollector(q *SourceQuerier, ts *hot.TimescaleLogger) *HAReplicaCollector {
	return &HAReplicaCollector{
		querier:       q,
		tsLogger:      ts,
		prevPrimaries: make(map[uuid.UUID]string),
	}
}

// Run fetches AG replica and database state and writes to TimescaleDB.
func (c *HAReplicaCollector) Run(ctx context.Context, serverID uuid.UUID, instanceName string) error {
	now := time.Now().UTC()

	// 1. Fetch Replica State
	replicas, err := c.querier.FetchReplicaHealth(ctx, instanceName)
	if err != nil {
		return err
	}

	// 1b. Fetch Extra Readiness Metrics
	longRunningCount, quorumState, _ := c.querier.FetchFailoverReadinessMetrics(ctx, instanceName)

	writeRows := make([]hot.HAReplicaStateRow, len(replicas))
	currPrimary := ""

	for i, r := range replicas {
		var lastCommit *time.Time
		if !r.LastCommitTime.IsZero() && r.LastCommitTime.Year() > 1970 {
			t := r.LastCommitTime
			lastCommit = &t
		}

		writeRows[i] = hot.HAReplicaStateRow{
			CaptureTimestamp:          now,
			ServerID:                  serverID,
			AGName:                    r.AGName,
			ReplicaServerName:         r.ReplicaServerName,
			RoleDesc:                  r.RoleDesc,
			SynchronizationStateDesc:  r.SynchronizationStateDesc,
			SynchronizationHealthDesc: r.SynchronizationHealth,
			AvailabilityModeDesc:      r.AvailabilityModeDesc,
			LogSendQueueKB:            r.LogSendQueueKB,
			RedoQueueKB:               r.RedoQueueKB,
			LogSendRateKBPS:           r.LogSendRateKBPS,
			RedoRateKBPS:              r.RedoRateKBPS,
			LastCommitTime:            lastCommit,
			SecondaryLagSeconds:       r.SecondaryLagSeconds,
			ConnectedStateDesc:        r.ConnectedStateDesc,
			IsFailoverReady:           r.IsFailoverReady,
			LongRunningTxCount:        longRunningCount,
			QuorumStateDesc:           quorumState,
		}

		if r.RoleDesc == "PRIMARY" {
			currPrimary = r.ReplicaServerName
		}
	}

	// 2. Detect Failover
	c.mu.Lock()
	prevPrimary := c.prevPrimaries[serverID]
	if prevPrimary != "" && currPrimary != "" && prevPrimary != currPrimary {
		slog.Error(fmt.Sprintf("[HA-FAILOVER] server=%s primary changed from %s to %s", instanceName, prevPrimary, currPrimary))
		_ = c.tsLogger.WriteHAFailoverEvent(ctx, hot.HAFailoverEventRow{
			CaptureTimestamp:        now,
			ServerID:                serverID,
			AGName:                  "ALL", // Simplified for now
			PreviousPrimary:         prevPrimary,
			NewPrimary:              currPrimary,
			FailoverType:            "AUTOMATIC",
			FailoverDurationSeconds: 0,
		})
	}
	c.prevPrimaries[serverID] = currPrimary
	c.mu.Unlock()

	// 3. Write Replica State
	if err := c.tsLogger.WriteHAReplicaState(ctx, writeRows); err != nil {
		return err
	}

	// 4. Fetch and Write Database Sync State
	dbStates, err := c.querier.FetchDatabaseSyncState(ctx, instanceName)
	if err != nil {
		slog.Error("[HA-Collector] failed to fetch DB sync state", "target", instanceName, "err", err)
		return nil // Non-fatal
	}

	// 4b. Fetch Backup Freshness
	backupFresh, _ := c.querier.FetchBackupFreshness(ctx, instanceName)

	dbRows := make([]hot.HADatabaseStateRow, len(dbStates))
	for i, d := range dbStates {
		var lastCommit *time.Time
		if !d.LastCommitTime.IsZero() && d.LastCommitTime.Year() > 1970 {
			t := d.LastCommitTime
			lastCommit = &t
		}
		dbRows[i] = hot.HADatabaseStateRow{
			CaptureTimestamp:         now,
			ServerID:                 serverID,
			AGName:                   d.AGName,
			DatabaseName:             d.DatabaseName,
			ReplicaServerName:        d.ReplicaServerName,
			SynchronizationStateDesc: d.SynchronizationStateDesc,
			IsSuspended:              d.IsSuspended,
			LogSendQueueKB:           d.LogSendQueueKB,
			RedoQueueKB:              d.RedoQueueKB,
			LastCommitTime:           lastCommit,
			BackupFreshOK:            backupFresh[d.DatabaseName],
		}
	}

	return c.tsLogger.WriteHADatabaseState(ctx, dbRows)
}
