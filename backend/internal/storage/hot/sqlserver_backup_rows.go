// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Timescale row types for SQL Server backup collectors (avoids import cycles).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"time"

	"github.com/google/uuid"
)

type BackupPostureHotRow struct {
	CaptureTimestamp         time.Time
	ServerID                 uuid.UUID
	DatabaseName             string
	RecoveryModelDesc        string
	LastFullFinish           *time.Time
	LastDiffFinish           *time.Time
	LastLogFinish            *time.Time
	MinutesSinceFull         int
	MinutesSinceLog          int
	LastFullSizeMB           float64
	LastLogSizeMB            float64
	FullFreshOK              bool
	LogFreshOK               bool
	HasFullBackup            bool
	BackupCompressionDefault bool
}

type BackupHistoryHotRow struct {
	CaptureTimestamp       time.Time
	ServerID               uuid.UUID
	BackupSetUUID          uuid.UUID
	DatabaseName           string
	BackupType             string
	BackupStartDate        *time.Time
	BackupFinishDate       *time.Time
	BackupSizeMB           float64
	CompressedBackupSizeMB float64
	DurationSeconds        int
	IsCopyOnly             bool
	HasChecksum            bool
	IsCompressed           bool
	FirstLSN               string
	LastLSN                string
	UserName               string
}
