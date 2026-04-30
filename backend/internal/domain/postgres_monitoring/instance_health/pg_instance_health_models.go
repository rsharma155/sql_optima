// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Models for PostgreSQL Instance Health.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package instance_health

import "time"

type PgInstanceSnapshot struct {
	InstanceID            string    `json:"instance_id"`
	CollectedAt           time.Time `json:"collected_at"`
	TPS                   float64   `json:"tps"`
	ActiveSessions        int       `json:"active_sessions"`
	IdleSessions          int       `json:"idle_sessions"`
	IdleInTxSessions      int       `json:"idle_in_tx_sessions"`
	BlockedSessions       int       `json:"blocked_sessions"`
	CPUUsage              float64   `json:"cpu_usage"`
	SharedBuffersUsedPct  float64   `json:"shared_buffers_used_pct"`
	CacheHitRatio         float64   `json:"cache_hit_ratio"`
	WALMBPerMin           float64   `json:"wal_mb_per_min"`
	CheckpointReqRatio    float64   `json:"checkpoint_req_ratio"`
	CheckpointsTimed      int       `json:"checkpoints_timed"`
	CheckpointsReq        int       `json:"checkpoints_req"`
	CheckpointWriteTime   float64   `json:"checkpoint_write_time"`
	MaxXIDAge             int64     `json:"max_xid_age"`
	OldestTxAgeSec        int64     `json:"oldest_tx_age_sec"`
	DatabaseSizeGB        float64   `json:"database_size_gb"`
	TempBytesMB           float64   `json:"temp_bytes_mb"`
	AutovacuumWorkers  int       `json:"autovacuum_workers"`
	DeadTuplePct       float64   `json:"dead_tuple_pct"`
	ReplicaLagSec      float64   `json:"replica_lag_sec"`
	ReplicationSlots   int       `json:"replication_slots"`
	HealthScore           int       `json:"health_score"`
	Version               string    `json:"version"`
	Uptime                string    `json:"uptime"`
}
