// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Backup & DR domain entities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package entities

import "github.com/google/uuid"

import "time"

// PGBackupDRSnapshot represents a unified snapshot of Backup & DR metrics.
type PGBackupDRSnapshot struct {
	CollectedAt           time.Time  `json:"capture_timestamp"`
	ServerID              uuid.UUID  `json:"server_id" db:"server_id"`
	WALBytesTotal         int64      `json:"wal_bytes_total"`
	WALRecordsTotal       int64      `json:"wal_records_total"`
	WALFPITotal           int64      `json:"wal_fpi_total"`
	ArchivedCount         int64      `json:"archived_count"`
	ArchiveFailedCount    int64      `json:"archive_failed_count"`
	LastArchivedTime      *time.Time `json:"last_archived_time"`
	LastFailedTime        *time.Time `json:"last_failed_time"`
	CheckpointsTimed      int64      `json:"checkpoints_timed"`
	CheckpointsReq        int64      `json:"checkpoints_req"`
	CheckpointWriteTimeMs float64    `json:"checkpoint_write_time_ms"`
	CheckpointSyncTimeMs  float64    `json:"checkpoint_sync_time_ms"`
	IsInRecovery          bool       `json:"is_in_recovery"`
}

// PGReplicationSnapshot represents a snapshot of a replication node.
type PGReplicationSnapshot struct {
	CollectedAt     time.Time `json:"capture_timestamp"`
	ServerID        uuid.UUID `json:"server_id" db:"server_id"`
	ApplicationName string    `json:"application_name"`
	ClientAddr      string    `json:"client_addr"`
	State           string    `json:"state"`
	SyncState       string    `json:"sync_state"`
	WriteLag        string    `json:"write_lag"`
	FlushLag        string    `json:"flush_lag"`
	ReplayLag       string    `json:"replay_lag"`
	SlotName        string    `json:"slot_name"`
	RetainedBytes   int64     `json:"retained_bytes"`
}

// ArchiveErrorLog represents an entry in the archiver error audit.
type ArchiveErrorLog struct {
	CollectedAt    time.Time `json:"capture_timestamp"`
	ServerID       uuid.UUID `json:"server_id" db:"server_id"`
	FailedCount    int64     `json:"failed_count"`
	LastFailedTime time.Time `json:"last_failed_time"`
}

// Deprecated entities (kept for backward compatibility during transition if needed, but we should eventually remove)

// BackupArchiverStats represents pg_stat_archiver metrics.
type BackupArchiverStats struct {
	TS               time.Time  `json:"capture_timestamp"`
	ServerID         uuid.UUID  `json:"server_id" db:"server_id"`
	ArchivedCount    int64      `json:"archived_count"`
	FailedCount      int64      `json:"failed_count"`
	LastArchivedTime *time.Time `json:"last_archived_time"`
	LastFailedTime   *time.Time `json:"last_failed_time"`
}

// WALRate represents WAL generation rate metrics.
type WALRate struct {
	TS       time.Time `json:"capture_timestamp"`
	ServerID uuid.UUID `json:"server_id" db:"server_id"`
	WALBytes float64   `json:"wal_bytes"`
}

// BaseBackupHistory represents base backup detection metrics.
type BaseBackupHistory struct {
	TS                  time.Time  `json:"capture_timestamp"`
	ServerID            uuid.UUID  `json:"server_id" db:"server_id"`
	CheckpointTime      *time.Time `json:"checkpoint_time"`
	CheckpointWriteTime float64    `json:"checkpoint_write_time"`
}
