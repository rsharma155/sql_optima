// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Core TimescaleDB logger for SQL Server metrics including CPU, memory, connections, locks.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TimescaleLogger struct {
	pool                    *pgxpool.Pool
	mu                      sync.RWMutex
	prevDiskHistory         map[string]map[string]interface{}
	prevJobDetails          map[string]map[string]interface{}
	prevJobFailures         map[string]string
	prevLongRunningHash     map[string]int64
	prevMemoryPLE           float64
	prevWaitHistory         map[uuid.UUID]map[string]float64
	prevQueryStoreStats     map[string]int64
	prevSchedulerStats      map[uuid.UUID]uint64
	prevEnterpriseBatchHash map[string]uint64 // Keep as string key for composite (serverID + kind)
	// Postgres Control Center dedup/delta state
	prevPgWalBytesTotal        map[uuid.UUID]uint64
	prevPgXactTotal            map[uuid.UUID]uint64
	prevPgControlCenterHash    map[uuid.UUID]uint64
	prevPgReplicationSlotsHash map[uuid.UUID]uint64
	prevPgDeadlocksTotal       map[uuid.UUID]map[string]int64 // serverID -> db -> last total
	prevPgDeadlocksTotalAllDBs map[uuid.UUID]int64            // serverID -> aggregate total
	prevPgWaitEventsHash       map[uuid.UUID]uint64
	prevPgDbIOHash             map[uuid.UUID]uint64
	prevPgSettingsHash         map[uuid.UUID]uint64
	prevPgQueryStatsHash       map[uuid.UUID]uint64
	prevPgStatDeltaHash        map[uuid.UUID]uint64
	prevPgSessionHash          map[uuid.UUID]uint64
	prevPgTblMaintHash         map[uuid.UUID]uint64
	prevPgVacuumProgressHash   map[uuid.UUID]uint64
	// Pulse Deduplication State
	prevSQLServerTier1Hash map[uuid.UUID]map[int]uint64 // serverID -> sid -> hash
	prevPgTier1Hash        map[uuid.UUID]map[int]uint64 // serverID -> pid -> hash
	// SQL Server Memory Analyzer delta state
	prevSpillByInstance map[uuid.UUID]spillDeltaState
	// SQL Server Performance Counter delta state
	prevPerfCounters map[uuid.UUID]map[string]float64
	prevPerfTime     map[uuid.UUID]time.Time
	// Dedup state for disk_history, file_io, wait_stats_cumulative, and connection_history
	prevDiskHashByKey map[string]uint64  // key: serverID+":"+dbName
	prevCumulSnapHash map[uuid.UUID]uint64
	prevFileIOHashKey          map[string]uint64 // key: serverID+":"+dbName+":"+fileName
	prevPerfCounterWriteHash   map[string]uint64 // key: serverID+":"+counterName+":"+instanceName
	prevConnHashByKey          map[string]uint64 // key: serverID+":"+dbName+":"+loginName
	prevVolumeHashByKey map[string]uint64 // key: serverID+":"+dbName+":"+logicalFileName
}

func NewTimescaleLogger(pool *pgxpool.Pool) *TimescaleLogger {
	return &TimescaleLogger{
		pool:                       pool,
		prevDiskHistory:            make(map[string]map[string]interface{}),
		prevJobDetails:             make(map[string]map[string]interface{}),
		prevJobFailures:            make(map[string]string),
		prevLongRunningHash:        make(map[string]int64),
		prevMemoryPLE:              -1,
		prevWaitHistory:            make(map[uuid.UUID]map[string]float64),
		prevQueryStoreStats:        make(map[string]int64),
		prevSchedulerStats:         make(map[uuid.UUID]uint64),
		prevEnterpriseBatchHash:    make(map[string]uint64),
		prevPgWalBytesTotal:        make(map[uuid.UUID]uint64),
		prevPgXactTotal:            make(map[uuid.UUID]uint64),
		prevPgControlCenterHash:    make(map[uuid.UUID]uint64),
		prevPgReplicationSlotsHash: make(map[uuid.UUID]uint64),
		prevPgDeadlocksTotal:       make(map[uuid.UUID]map[string]int64),
		prevPgDeadlocksTotalAllDBs: make(map[uuid.UUID]int64),
		prevPgWaitEventsHash:       make(map[uuid.UUID]uint64),
		prevPgDbIOHash:             make(map[uuid.UUID]uint64),
		prevPgSettingsHash:         make(map[uuid.UUID]uint64),
		prevPgQueryStatsHash:       make(map[uuid.UUID]uint64),
		prevPgStatDeltaHash:        make(map[uuid.UUID]uint64),
		prevPgSessionHash:          make(map[uuid.UUID]uint64),
		prevPgTblMaintHash:         make(map[uuid.UUID]uint64),
		prevPgVacuumProgressHash:   make(map[uuid.UUID]uint64),
		prevSQLServerTier1Hash:     make(map[uuid.UUID]map[int]uint64),
		prevPgTier1Hash:            make(map[uuid.UUID]map[int]uint64),
		prevSpillByInstance:        make(map[uuid.UUID]spillDeltaState),
		prevPerfCounters:           make(map[uuid.UUID]map[string]float64),
		prevPerfTime:               make(map[uuid.UUID]time.Time),
		prevDiskHashByKey:          make(map[string]uint64),
		prevCumulSnapHash:          make(map[uuid.UUID]uint64),
		prevFileIOHashKey:          make(map[string]uint64),
		prevPerfCounterWriteHash:   make(map[string]uint64),
		prevConnHashByKey:          make(map[string]uint64),
		prevVolumeHashByKey:        make(map[string]uint64),
	}
}

func (tl *TimescaleLogger) Pool() *pgxpool.Pool {
	tl.mu.RLock()
	defer tl.mu.RUnlock()
	return tl.pool
}

// GetPrevWaitHistory returns the cached wait history for an instance.
func (tl *TimescaleLogger) GetPrevWaitHistory(serverID uuid.UUID) map[string]float64 {
	tl.mu.RLock()
	defer tl.mu.RUnlock()
	return tl.prevWaitHistory[serverID]
}
