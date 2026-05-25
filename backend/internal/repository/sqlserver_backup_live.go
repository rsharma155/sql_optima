// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Live DMV queries for SQL Server backup posture and history (msdb.dbo.backupset).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	backupdomain "github.com/rsharma155/sql_optima/internal/domain/sqlserver_backup_recovery/domain"
	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

const sqlserverBackupPostureQuery = `
	/* SQL_OPTIMA — sqlserver_backup_posture */
	DECLARE @compress BIT = (
		SELECT CAST(ISNULL(value_in_use, 0) AS BIT)
		FROM sys.configurations WITH (NOLOCK)
		WHERE name = 'backup compression default'
	);
	WITH dbs AS (
		SELECT name, recovery_model_desc
		FROM sys.databases WITH (NOLOCK)
		WHERE database_id > 4 AND state = 0 AND source_database_id IS NULL
	),
	full_b AS (
		SELECT database_name,
			MAX(backup_finish_date) AS last_finish,
			MAX(CASE WHEN rn = 1 THEN compressed_backup_size END) / 1048576.0 AS size_mb
		FROM (
			SELECT database_name, backup_finish_date, compressed_backup_size,
				ROW_NUMBER() OVER (PARTITION BY database_name ORDER BY backup_finish_date DESC) AS rn
			FROM msdb.dbo.backupset WITH (NOLOCK)
			WHERE type = 'D'
		) x
		GROUP BY database_name
	),
	diff_b AS (
		SELECT database_name, MAX(backup_finish_date) AS last_finish
		FROM msdb.dbo.backupset WITH (NOLOCK)
		WHERE type = 'I'
		GROUP BY database_name
	),
	log_b AS (
		SELECT database_name,
			MAX(backup_finish_date) AS last_finish,
			MAX(CASE WHEN rn = 1 THEN compressed_backup_size END) / 1048576.0 AS size_mb
		FROM (
			SELECT database_name, backup_finish_date, compressed_backup_size,
				ROW_NUMBER() OVER (PARTITION BY database_name ORDER BY backup_finish_date DESC) AS rn
			FROM msdb.dbo.backupset WITH (NOLOCK)
			WHERE type = 'L'
		) x
		GROUP BY database_name
	)
	SELECT
		d.name,
		d.recovery_model_desc,
		f.last_finish,
		di.last_finish,
		l.last_finish,
		CASE WHEN f.last_finish IS NULL THEN 999999
		     ELSE DATEDIFF(MINUTE, f.last_finish, GETUTCDATE()) END,
		CASE WHEN l.last_finish IS NULL THEN 999999
		     ELSE DATEDIFF(MINUTE, l.last_finish, GETUTCDATE()) END,
		ISNULL(f.size_mb, 0),
		ISNULL(l.size_mb, 0),
		CASE WHEN f.last_finish IS NOT NULL THEN 1 ELSE 0 END,
		@compress
	FROM dbs d
	LEFT JOIN full_b f ON d.name = f.database_name
	LEFT JOIN diff_b di ON d.name = di.database_name
	LEFT JOIN log_b l ON d.name = l.database_name
	ORDER BY d.name;
`

const sqlserverBackupHistoryQuery = `
	/* SQL_OPTIMA — sqlserver_backup_history */
	SELECT TOP (%d)
		bs.backup_set_uuid,
		bs.database_name,
		bs.type,
		bs.backup_start_date,
		bs.backup_finish_date,
		bs.backup_size / 1048576.0,
		bs.compressed_backup_size / 1048576.0,
		DATEDIFF(SECOND, bs.backup_start_date, bs.backup_finish_date),
		bs.is_copy_only,
		bs.has_backup_checksum,
		CASE
			WHEN bs.compressed_backup_size IS NOT NULL
				AND bs.backup_size > 0
				AND bs.compressed_backup_size < bs.backup_size THEN 1
			ELSE 0
		END,
		ISNULL(CONVERT(VARCHAR(64), bs.first_lsn), ''),
		ISNULL(CONVERT(VARCHAR(64), bs.last_lsn), ''),
		ISNULL(bs.user_name, '')
	FROM msdb.dbo.backupset bs WITH (NOLOCK)
	WHERE bs.backup_finish_date >= DATEADD(DAY, -7, GETUTCDATE())
	ORDER BY bs.backup_finish_date DESC;
`

func (c *SqlServerRepository) FetchBackupPostureLive(ctx context.Context, instanceName string) ([]backupdomain.DatabasePosture, bool, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, false, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	ctx, cancel := WithQueryTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, sqlserverBackupPostureQuery)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	var out []backupdomain.DatabasePosture
	var compressionDefault bool
	for rows.Next() {
		var p backupdomain.DatabasePosture
		var lastFull, lastDiff, lastLog sql.NullTime
		var hasFull int
		if err := rows.Scan(
			&p.DatabaseName, &p.RecoveryModel,
			&lastFull, &lastDiff, &lastLog,
			&p.MinutesSinceFull, &p.MinutesSinceLog,
			&p.LastFullSizeMB, &p.LastLogSizeMB,
			&hasFull, &compressionDefault,
		); err != nil {
			continue
		}
		p.HasFullBackup = hasFull == 1
		p.BackupCompressionDefault = compressionDefault
		if lastFull.Valid {
			t := lastFull.Time.UTC()
			p.LastFullFinish = &t
		}
		if lastDiff.Valid {
			t := lastDiff.Time.UTC()
			p.LastDiffFinish = &t
		}
		if lastLog.Valid {
			t := lastLog.Time.UTC()
			p.LastLogFinish = &t
		}
		if p.MinutesSinceFull < 0 || p.MinutesSinceFull > 525600 {
			p.MinutesSinceFull = 999999
		}
		if p.MinutesSinceLog < 0 || p.MinutesSinceLog > 525600 {
			p.MinutesSinceLog = 999999
		}
		out = append(out, p)
	}
	return out, compressionDefault, rows.Err()
}

func (c *SqlServerRepository) FetchBackupHistoryLive(ctx context.Context, instanceName string, limit int) ([]backupdomain.BackupHistoryRow, error) {
	if limit <= 0 {
		limit = 500
	}
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("no connection for instance: %s", instanceName)
	}
	q := fmt.Sprintf(sqlserverBackupHistoryQuery, limit)
	ctx, cancel := WithQueryTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []backupdomain.BackupHistoryRow
	for rows.Next() {
		var h backupdomain.BackupHistoryRow
		var btype string
		var start, finish sql.NullTime
		if err := rows.Scan(
			&h.BackupSetUUID, &h.DatabaseName, &btype,
			&start, &finish,
			&h.BackupSizeMB, &h.CompressedBackupSizeMB,
			&h.DurationSeconds,
			&h.IsCopyOnly, &h.HasChecksum, &h.IsCompressed,
			&h.FirstLSN, &h.LastLSN, &h.UserName,
		); err != nil {
			continue
		}
		h.BackupType = btype
		if start.Valid {
			t := start.Time.UTC()
			h.BackupStartDate = &t
		}
		if finish.Valid {
			t := finish.Time.UTC()
			h.BackupFinishDate = &t
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// MapPostureToHotRows converts domain posture to hot logger rows at capture time.
func MapPostureToHotRows(serverID uuid.UUID, capture time.Time, posture []backupdomain.DatabasePosture, compressionDefault bool) []hot.BackupPostureHotRow {
	rows := make([]hot.BackupPostureHotRow, len(posture))
	for i, p := range posture {
		rows[i] = hot.BackupPostureHotRow{
			CaptureTimestamp:         capture,
			ServerID:                 serverID,
			DatabaseName:             p.DatabaseName,
			RecoveryModelDesc:        p.RecoveryModel,
			LastFullFinish:           p.LastFullFinish,
			LastDiffFinish:           p.LastDiffFinish,
			LastLogFinish:            p.LastLogFinish,
			MinutesSinceFull:         p.MinutesSinceFull,
			MinutesSinceLog:          p.MinutesSinceLog,
			LastFullSizeMB:           p.LastFullSizeMB,
			LastLogSizeMB:            p.LastLogSizeMB,
			FullFreshOK:              p.FullFreshOK,
			LogFreshOK:               p.LogFreshOK,
			HasFullBackup:            p.HasFullBackup,
			BackupCompressionDefault: compressionDefault,
		}
	}
	return rows
}

func MapHistoryToHotRows(serverID uuid.UUID, capture time.Time, history []backupdomain.BackupHistoryRow) []hot.BackupHistoryHotRow {
	rows := make([]hot.BackupHistoryHotRow, len(history))
	for i, h := range history {
		rows[i] = hot.BackupHistoryHotRow{
			CaptureTimestamp:       capture,
			ServerID:               serverID,
			BackupSetUUID:          h.BackupSetUUID,
			DatabaseName:           h.DatabaseName,
			BackupType:             h.BackupType,
			BackupStartDate:        h.BackupStartDate,
			BackupFinishDate:       h.BackupFinishDate,
			BackupSizeMB:           h.BackupSizeMB,
			CompressedBackupSizeMB: h.CompressedBackupSizeMB,
			DurationSeconds:        h.DurationSeconds,
			IsCopyOnly:             h.IsCopyOnly,
			HasChecksum:            h.HasChecksum,
			IsCompressed:           h.IsCompressed,
			FirstLSN:               h.FirstLSN,
			LastLSN:                h.LastLSN,
			UserName:               h.UserName,
		}
	}
	return rows
}
