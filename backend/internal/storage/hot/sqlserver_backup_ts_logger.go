// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB writers and readers for SQL Server backup posture and history.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

func isBackupSchemaMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "undefined_table")
}

func (tl *TimescaleLogger) LogSQLServerBackupPosture(ctx context.Context, rows []BackupPostureHotRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.sqlserver_backup_database_posture (
			capture_timestamp, server_id, database_name, recovery_model_desc,
			last_full_finish, last_diff_finish, last_log_finish,
			minutes_since_full, minutes_since_log,
			last_full_size_mb, last_log_size_mb,
			full_fresh_ok, log_fresh_ok, has_full_backup, backup_compression_default
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`
	for _, r := range rows {
		if _, err := tl.pool.Exec(ctx, q,
			r.CaptureTimestamp, r.ServerID, r.DatabaseName, r.RecoveryModelDesc,
			r.LastFullFinish, r.LastDiffFinish, r.LastLogFinish,
			r.MinutesSinceFull, r.MinutesSinceLog,
			r.LastFullSizeMB, r.LastLogSizeMB,
			r.FullFreshOK, r.LogFreshOK, r.HasFullBackup, r.BackupCompressionDefault,
		); err != nil {
			return fmt.Errorf("backup posture insert %s: %w", r.DatabaseName, err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogSQLServerBackupHistory(ctx context.Context, rows []BackupHistoryHotRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO monitor.sqlserver_backup_history (
			capture_timestamp, server_id, backup_set_uuid, database_name, backup_type,
			backup_start_date, backup_finish_date,
			backup_size_mb, compressed_backup_size_mb, duration_seconds,
			is_copy_only, has_checksum, is_compressed,
			first_lsn, last_lsn, user_name, physical_device
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'')
		ON CONFLICT DO NOTHING`
	for _, r := range rows {
		if _, err := tl.pool.Exec(ctx, q,
			r.CaptureTimestamp, r.ServerID, r.BackupSetUUID, r.DatabaseName, r.BackupType,
			r.BackupStartDate, r.BackupFinishDate,
			r.BackupSizeMB, r.CompressedBackupSizeMB, r.DurationSeconds,
			r.IsCopyOnly, r.HasChecksum, r.IsCompressed,
			r.FirstLSN, r.LastLSN, r.UserName,
		); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetLatestSQLServerBackupPosture(ctx context.Context, serverID uuid.UUID) ([]map[string]interface{}, error) {
	const q = `
		SELECT DISTINCT ON (database_name)
			database_name, recovery_model_desc,
			last_full_finish, last_diff_finish, last_log_finish,
			minutes_since_full, minutes_since_log,
			last_full_size_mb, last_log_size_mb,
			full_fresh_ok, log_fresh_ok, has_full_backup, backup_compression_default,
			capture_timestamp
		FROM monitor.sqlserver_backup_database_posture
		WHERE server_id = $1
		  AND capture_timestamp > now() - INTERVAL '48 hours'
		ORDER BY database_name, capture_timestamp DESC`
	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		if isBackupSchemaMissing(err) {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	var results []map[string]interface{}
	for rows.Next() {
		var (
			dbName, recovery string
			lastFull, lastDiff, lastLog *time.Time
			minFull, minLog int
			fullMB, logMB float64
			fullOK, logOK, hasFull, compress bool
			capture time.Time
		)
		if err := rows.Scan(
			&dbName, &recovery, &lastFull, &lastDiff, &lastLog,
			&minFull, &minLog, &fullMB, &logMB,
			&fullOK, &logOK, &hasFull, &compress, &capture,
		); err != nil {
			continue
		}
		row := map[string]interface{}{
			"database_name": dbName, "recovery_model_desc": recovery,
			"minutes_since_full": minFull, "minutes_since_log": minLog,
			"last_full_size_mb": fullMB, "last_log_size_mb": logMB,
			"full_fresh_ok": fullOK, "log_fresh_ok": logOK,
			"has_full_backup": hasFull, "backup_compression_default": compress,
			"capture_timestamp": capture,
		}
		if lastFull != nil {
			row["last_full_finish"] = *lastFull
		}
		if lastDiff != nil {
			row["last_diff_finish"] = *lastDiff
		}
		if lastLog != nil {
			row["last_log_finish"] = *lastLog
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerBackupHistory(ctx context.Context, serverID uuid.UUID, from, to time.Time, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 200
	}
	const q = `
		SELECT DISTINCT ON (backup_set_uuid)
			backup_set_uuid, database_name, backup_type,
			backup_start_date, backup_finish_date,
			backup_size_mb, compressed_backup_size_mb, duration_seconds,
			is_copy_only, has_checksum, is_compressed,
			first_lsn, last_lsn, user_name
		FROM monitor.sqlserver_backup_history
		WHERE server_id = $1
		  AND backup_finish_date >= $2
		  AND backup_finish_date <= $3
		ORDER BY backup_set_uuid, backup_finish_date DESC
		LIMIT $4`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to, limit)
	if err != nil {
		if isBackupSchemaMissing(err) {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	var results []map[string]interface{}
	for rows.Next() {
		var (
			bsUUID uuid.UUID
			dbName, btype, firstLSN, lastLSN, user string
			start, finish *time.Time
			sizeMB, compMB float64
			dur int
			copyOnly, checksum, compressed bool
		)
		if err := rows.Scan(
			&bsUUID, &dbName, &btype, &start, &finish,
			&sizeMB, &compMB, &dur,
			&copyOnly, &checksum, &compressed,
			&firstLSN, &lastLSN, &user,
		); err != nil {
			continue
		}
		row := map[string]interface{}{
			"backup_set_uuid": bsUUID, "database_name": dbName, "backup_type": btype,
			"backup_size_mb": sizeMB, "compressed_backup_size_mb": compMB,
			"duration_seconds": dur,
			"is_copy_only": copyOnly, "has_checksum": checksum, "is_compressed": compressed,
			"first_lsn": firstLSN, "last_lsn": lastLSN, "user_name": user,
		}
		if start != nil {
			row["backup_start_date"] = *start
		}
		if finish != nil {
			row["backup_finish_date"] = *finish
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (tl *TimescaleLogger) GetSQLServerBackupHistoryTrend(ctx context.Context, serverID uuid.UUID, from, to time.Time) ([]map[string]interface{}, error) {
	const q = `
		SELECT
			time_bucket('1 hour', backup_finish_date) AS bucket,
			SUM(CASE WHEN backup_type = 'D' THEN backup_size_mb ELSE 0 END) AS full_mb,
			SUM(CASE WHEN backup_type = 'I' THEN backup_size_mb ELSE 0 END) AS diff_mb,
			SUM(CASE WHEN backup_type = 'L' THEN backup_size_mb ELSE 0 END) AS log_mb,
			COUNT(*)::int AS backup_count
		FROM monitor.sqlserver_backup_history
		WHERE server_id = $1
		  AND backup_finish_date >= $2
		  AND backup_finish_date <= $3
		GROUP BY 1
		ORDER BY 1`
	rows, err := tl.pool.Query(ctx, q, serverID, from, to)
	if err != nil {
		if isBackupSchemaMissing(err) {
			return []map[string]interface{}{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	var results []map[string]interface{}
	for rows.Next() {
		var bucket time.Time
		var fullMB, diffMB, logMB float64
		var count int
		if err := rows.Scan(&bucket, &fullMB, &diffMB, &logMB, &count); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"timestamp": bucket, "full_mb": fullMB, "diff_mb": diffMB,
			"log_mb": logMB, "backup_count": count,
		})
	}
	return results, rows.Err()
}
