// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage-layer for SQL Server vitals (volume stats).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// LogVolumeStats persists per-volume storage metrics with delta-check logic.
func (tl *TimescaleLogger) LogVolumeStats(ctx context.Context, serverID uuid.UUID, rows []VolumeStatsRow) error {
	if len(rows) == 0 {
		return nil
	}

	var toInsert []VolumeStatsRow
	tl.mu.Lock()
	for _, r := range rows {
		key := serverID.String() + ":" + r.DatabaseName + ":" + r.LogicalFileName
		newHash := volumeStatsHash(r.VolumeAvailableGB, r.VolumeFreePct)
		if prev, ok := tl.prevVolumeHashByKey[key]; ok && prev == newHash {
			continue // skip if unchanged
		}
		tl.prevVolumeHashByKey[key] = newHash
		toInsert = append(toInsert, r)
	}
	tl.mu.Unlock()

	if len(toInsert) == 0 {
		return nil
	}

	now := time.Now().UTC()
	q := `
		INSERT INTO sqlserver_volume_stats (
			capture_timestamp, server_id, database_name, logical_file_name,
			physical_name, file_type, file_size_mb, volume_mount_point,
			volume_label, volume_total_gb, volume_available_gb, volume_free_pct
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	batch := &pgx.Batch{}
	for _, r := range toInsert {
		batch.Queue(q,
			now, serverID, r.DatabaseName, r.LogicalFileName,
			r.PhysicalName, r.FileType, r.FileSizeMB, r.VolumeMountPoint,
			r.VolumeLabel, r.VolumeTotalGB, r.VolumeAvailableGB, r.VolumeFreePct,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	return br.Close()
}

// GetLatestSQLServerMemoryMetrics retrieves the most recent memory snapshot.
func (tl *TimescaleLogger) GetLatestSQLServerMemoryMetrics(ctx context.Context, serverID uuid.UUID) (map[string]interface{}, error) {
	q := `
		SELECT 
			sql_memory_used_mb, sql_memory_target_mb,
			os_total_memory_mb, os_available_memory_mb,
			os_system_memory_state,
			process_physical_low, process_virtual_low,
			memory_grants_pending, active_memory_grants, waiting_memory_grants,
			granted_workspace_mb, requested_workspace_mb,
			ple_seconds, plan_cache_mb,
			sort_warnings_per_sec, hash_warnings_per_sec,
			sql_physical_memory_in_use_mb, sql_memory_utilization_pct,
			sql_page_fault_count, sql_locked_page_alloc_mb, sql_large_page_alloc_mb,
			capture_timestamp
		FROM sqlserver_memory_metrics
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}

	values, err := rows.Values()
	if err != nil {
		return nil, err
	}

	cols := []string{
		"sql_memory_used_mb", "sql_memory_target_mb",
		"os_total_memory_mb", "os_available_memory_mb",
		"os_system_memory_state",
		"process_physical_low", "process_virtual_low",
		"memory_grants_pending", "active_memory_grants", "waiting_memory_grants",
		"granted_workspace_mb", "requested_workspace_mb",
		"ple_seconds", "plan_cache_mb",
		"sort_warnings_per_sec", "hash_warnings_per_sec",
		"sql_physical_memory_in_use_mb", "sql_memory_utilization_pct",
		"sql_page_fault_count", "sql_locked_page_alloc_mb", "sql_large_page_alloc_mb",
		"capture_timestamp",
	}

	out := make(map[string]interface{})
	for i, col := range cols {
		out[col] = values[i]
	}
	return out, nil
}

// GetLatestSQLServerVolumeStats retrieves the most recent volume snapshot.
func (tl *TimescaleLogger) GetLatestSQLServerVolumeStats(ctx context.Context, serverID uuid.UUID) ([]VolumeStatsRow, error) {
	q := `
		WITH latest_ts AS (
			SELECT MAX(capture_timestamp) as ts
			FROM sqlserver_volume_stats
			WHERE server_id = $1
		)
		SELECT 
			capture_timestamp, database_name, logical_file_name,
			physical_name, file_type, file_size_mb, volume_mount_point,
			volume_label, volume_total_gb, volume_available_gb, volume_free_pct
		FROM sqlserver_volume_stats
		WHERE server_id = $1
		  AND capture_timestamp = (SELECT ts FROM latest_ts)
		ORDER BY volume_free_pct ASC
	`
	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []VolumeStatsRow
	for rows.Next() {
		var r VolumeStatsRow
		r.ServerID = serverID
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.DatabaseName, &r.LogicalFileName,
			&r.PhysicalName, &r.FileType, &r.FileSizeMB, &r.VolumeMountPoint,
			&r.VolumeLabel, &r.VolumeTotalGB, &r.VolumeAvailableGB, &r.VolumeFreePct,
		); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
