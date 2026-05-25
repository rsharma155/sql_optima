// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Assembles SQL Server Backup & Recovery dashboard data from TimescaleDB.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	backupdomain "github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/domain"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

type SQLServerBackupDashboardRepository struct {
	logger *hot.TimescaleLogger
}

func NewSQLServerBackupDashboardRepository(logger *hot.TimescaleLogger) *SQLServerBackupDashboardRepository {
	return &SQLServerBackupDashboardRepository{logger: logger}
}

func (r *SQLServerBackupDashboardRepository) BuildKPIs(posture []backupdomain.DatabasePosture, history []backupdomain.BackupHistoryRow, failedJobs int) map[string]interface{} {
	staleFull := 0
	overdueLog := 0
	for _, p := range posture {
		if !p.FullFreshOK {
			staleFull++
		}
		if !p.LogFreshOK {
			overdueLog++
		}
	}
	var gb24h float64
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	for _, h := range history {
		if h.BackupFinishDate != nil && h.BackupFinishDate.After(cutoff) {
			gb24h += h.BackupSizeMB / 1024.0
		}
	}
	var totalDur, count int
	for _, h := range history {
		if h.BackupFinishDate != nil && h.BackupFinishDate.After(cutoff) {
			totalDur += h.DurationSeconds
			count++
		}
	}
	avgDur := 0
	if count > 0 {
		avgDur = totalDur / count
	}
	return map[string]interface{}{
		"databases_tracked":        len(posture),
		"stale_full":               staleFull,
		"overdue_log":              overdueLog,
		"failed_backup_jobs_24h":   failedJobs,
		"backup_gb_24h":            round2(gb24h),
		"avg_backup_duration_sec":  avgDur,
	}
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

func MapsToPosture(rows []map[string]interface{}) []backupdomain.DatabasePosture {
	out := make([]backupdomain.DatabasePosture, 0, len(rows))
	for _, m := range rows {
		p := backupdomain.DatabasePosture{
			DatabaseName:             str(m, "database_name"),
			RecoveryModel:            str(m, "recovery_model_desc"),
			MinutesSinceFull:         intVal(m, "minutes_since_full"),
			MinutesSinceLog:          intVal(m, "minutes_since_log"),
			LastFullSizeMB:           floatVal(m, "last_full_size_mb"),
			LastLogSizeMB:            floatVal(m, "last_log_size_mb"),
			FullFreshOK:              boolVal(m, "full_fresh_ok"),
			LogFreshOK:               boolVal(m, "log_fresh_ok"),
			HasFullBackup:            boolVal(m, "has_full_backup"),
			BackupCompressionDefault: boolVal(m, "backup_compression_default"),
		}
		p.LastFullFinish = timePtr(m, "last_full_finish")
		p.LastDiffFinish = timePtr(m, "last_diff_finish")
		p.LastLogFinish = timePtr(m, "last_log_finish")
		out = append(out, p)
	}
	return out
}

func MapsToHistory(rows []map[string]interface{}) []backupdomain.BackupHistoryRow {
	out := make([]backupdomain.BackupHistoryRow, 0, len(rows))
	for _, m := range rows {
		id, _ := m["backup_set_uuid"].(uuid.UUID)
		h := backupdomain.BackupHistoryRow{
			BackupSetUUID:          id,
			DatabaseName:           str(m, "database_name"),
			BackupType:             str(m, "backup_type"),
			BackupSizeMB:           floatVal(m, "backup_size_mb"),
			CompressedBackupSizeMB: floatVal(m, "compressed_backup_size_mb"),
			DurationSeconds:        intVal(m, "duration_seconds"),
			IsCopyOnly:             boolVal(m, "is_copy_only"),
			HasChecksum:            boolVal(m, "has_checksum"),
			IsCompressed:           boolVal(m, "is_compressed"),
			FirstLSN:               str(m, "first_lsn"),
			LastLSN:                str(m, "last_lsn"),
			UserName:               str(m, "user_name"),
		}
		h.BackupStartDate = timePtr(m, "backup_start_date")
		h.BackupFinishDate = timePtr(m, "backup_finish_date")
		out = append(out, h)
	}
	return out
}

func str(m map[string]interface{}, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func intVal(m map[string]interface{}, k string) int {
	switch v := m[k].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func floatVal(m map[string]interface{}, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}

func boolVal(m map[string]interface{}, k string) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return false
}

func timePtr(m map[string]interface{}, k string) *time.Time {
	switch v := m[k].(type) {
	case time.Time:
		t := v.UTC()
		return &t
	case *time.Time:
		if v == nil {
			return nil
		}
		t := v.UTC()
		return &t
	default:
		return nil
	}
}

func (r *SQLServerBackupDashboardRepository) GetPostureMaps(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	if r.logger == nil {
		return nil, nil
	}
	return r.logger.GetLatestSQLServerBackupPosture(ctx, serverID)
}

func (r *SQLServerBackupDashboardRepository) GetHistoryMaps(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if r.logger == nil {
		return nil, nil
	}
	return r.logger.GetSQLServerBackupHistory(ctx, serverID, from, to, limit)
}

func (r *SQLServerBackupDashboardRepository) GetHistoryTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	if r.logger == nil {
		return nil, nil
	}
	return r.logger.GetSQLServerBackupHistoryTrend(ctx, serverID, from, to)
}
