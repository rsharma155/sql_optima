// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Domain types for SQL Server Backup & Recovery dashboard.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package domain

import (
	"time"

	"github.com/google/uuid"
)

// BackupPolicy holds per-instance RPO thresholds (stored in optima_server_dr_policy).
type BackupPolicy struct {
	ServerID             uuid.UUID `json:"server_id"`
	RPOFullBackupHours   int       `json:"rpo_full_backup_hours"`
	RPOLogBackupMinutes  int       `json:"rpo_log_backup_minutes"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
	UpdatedBy            string    `json:"updated_by,omitempty"`
}

func DefaultBackupPolicy(serverID uuid.UUID) BackupPolicy {
	return BackupPolicy{
		ServerID:            serverID,
		RPOFullBackupHours:  24,
		RPOLogBackupMinutes: 15,
	}
}

// DatabasePosture is one row of per-database backup posture.
type DatabasePosture struct {
	DatabaseName             string     `json:"database_name"`
	RecoveryModel            string     `json:"recovery_model"`
	LastFullFinish           *time.Time `json:"last_full_finish,omitempty"`
	LastDiffFinish           *time.Time `json:"last_diff_finish,omitempty"`
	LastLogFinish            *time.Time `json:"last_log_finish,omitempty"`
	MinutesSinceFull         int        `json:"minutes_since_full"`
	MinutesSinceLog          int        `json:"minutes_since_log"`
	LastFullSizeMB           float64    `json:"last_full_size_mb"`
	LastLogSizeMB            float64    `json:"last_log_size_mb"`
	FullFreshOK              bool       `json:"full_fresh_ok"`
	LogFreshOK               bool       `json:"log_fresh_ok"`
	HasFullBackup            bool       `json:"has_full_backup"`
	BackupCompressionDefault bool       `json:"backup_compression_default"`
	ProtectionLevel          string     `json:"protection_level,omitempty"`
	InHA                     bool       `json:"in_ha,omitempty"`
}

// BackupHistoryRow is one backup operation from msdb.dbo.backupset.
type BackupHistoryRow struct {
	BackupSetUUID           uuid.UUID  `json:"backup_set_uuid"`
	DatabaseName            string     `json:"database_name"`
	BackupType              string     `json:"backup_type"`
	BackupStartDate         *time.Time `json:"backup_start_date,omitempty"`
	BackupFinishDate        *time.Time `json:"backup_finish_date,omitempty"`
	BackupSizeMB            float64    `json:"backup_size_mb"`
	CompressedBackupSizeMB  float64    `json:"compressed_backup_size_mb"`
	DurationSeconds         int        `json:"duration_seconds"`
	IsCopyOnly              bool       `json:"is_copy_only"`
	HasChecksum             bool       `json:"has_checksum"`
	IsCompressed            bool       `json:"is_compressed"`
	FirstLSN                string     `json:"first_lsn"`
	LastLSN                 string     `json:"last_lsn"`
	UserName                string     `json:"user_name"`
}

// ReadinessChip is one DR readiness indicator for the UI.
type ReadinessChip struct {
	Label string `json:"label"`
	Class string `json:"class"` // ok | warn | bad
}

// ReadinessSummary aggregates posture into an executive summary.
type ReadinessSummary struct {
	Overall string          `json:"overall"`
	Chips   []ReadinessChip `json:"chips"`
}

// DashboardPayload is the unified API response for the backup dashboard.
type DashboardPayload struct {
	Readiness      ReadinessSummary       `json:"readiness"`
	KPIs           map[string]interface{} `json:"kpis"`
	Posture        []DatabasePosture      `json:"posture"`
	History        []BackupHistoryRow     `json:"history"`
	HistoryTrend   []map[string]interface{} `json:"history_trend"`
	BackupJobs     map[string]interface{} `json:"backup_jobs"`
	LogShipping    []map[string]interface{} `json:"log_shipping"`
	HAContext      map[string]interface{} `json:"ha_context"`
	InstanceConfig map[string]interface{} `json:"instance_config"`
}
