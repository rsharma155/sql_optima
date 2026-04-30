// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL Backup & DR domain entities.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package entities

import "time"

// BackupArchiverStats represents pg_stat_archiver metrics.
type BackupArchiverStats struct {
	TS               time.Time `json:"ts"`
	InstanceID       string    `json:"instance_id"`
	ArchivedCount    int64     `json:"archived_count"`
	FailedCount      int64     `json:"failed_count"`
	LastArchivedTime *time.Time `json:"last_archived_time"`
	LastFailedTime   *time.Time `json:"last_failed_time"`
}

// WALRate represents WAL generation rate metrics.
type WALRate struct {
	TS         time.Time `json:"ts"`
	InstanceID string    `json:"instance_id"`
	WALBytes   float64   `json:"wal_bytes"`
}

// BaseBackupHistory represents base backup detection metrics.
type BaseBackupHistory struct {
	TS                  time.Time `json:"ts"`
	InstanceID          string    `json:"instance_id"`
	CheckpointTime      *time.Time `json:"checkpoint_time"`
	CheckpointWriteTime float64   `json:"checkpoint_write_time"`
}
