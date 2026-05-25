// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server volume and storage metrics collection.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/rsharma155/sql_optima/internal/storage/hot"
)

// FetchVolumeStats collects per-volume storage statistics using CROSS APPLY sys.dm_os_volume_stats.
func (c *SqlServerRepository) FetchVolumeStats(ctx context.Context, instanceName string) ([]hot.VolumeStatsRow, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */
		SELECT DISTINCT
			mf.database_id,
			DB_NAME(mf.database_id)            AS database_name,
			mf.name                            AS logical_file_name,
			mf.physical_name,
			mf.type_desc                       AS file_type,
			mf.size * 8 / 1024.0              AS file_size_mb,
			vs.volume_mount_point,
			vs.logical_volume_name             AS volume_label,
			vs.total_bytes    / 1073741824.0   AS volume_total_gb,
			vs.available_bytes / 1073741824.0  AS volume_available_gb,
			CAST(vs.available_bytes * 100.0
				 / NULLIF(vs.total_bytes, 0)
				 AS DECIMAL(5,2))              AS volume_free_pct
		FROM sys.master_files mf WITH (NOLOCK)
		CROSS APPLY sys.dm_os_volume_stats(mf.database_id, mf.file_id) vs
		WHERE mf.state = 0
		  AND mf.database_id > 4
		ORDER BY volume_free_pct ASC;
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []hot.VolumeStatsRow
	now := time.Now().UTC()

	for rows.Next() {
		var r hot.VolumeStatsRow
		r.CaptureTimestamp = now
		var dbID int
		if err := rows.Scan(
			&dbID, &r.DatabaseName, &r.LogicalFileName, &r.PhysicalName,
			&r.FileType, &r.FileSizeMB, &r.VolumeMountPoint, &r.VolumeLabel,
			&r.VolumeTotalGB, &r.VolumeAvailableGB, &r.VolumeFreePct,
		); err != nil {
			continue
		}
		results = append(results, r)
	}

	return results, rows.Err()
}
