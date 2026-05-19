// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Repository methods for PostgreSQL memory intelligence.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectPgHostMemory is intentionally a no-op when the OS collector agent is not running.
// Host memory data is written by the separate os_collector agent into monitor.pg_os_memory_samples.
// Returning an error causes the service to skip writing zeros to host_memory_samples,
// keeping the table clean. The frontend handles total_mem_mb==0 by showing "N/A".
func (c *PgRepository) CollectPgHostMemory(_ string) (*models.PgHostMemorySnapshot, error) {
	return nil, fmt.Errorf("host memory requires os_collector agent")
}

func (c *PgRepository) CollectPgMemoryStats(ctx context.Context, instanceName string) (*models.PgMemoryStatsSnapshot, error) {
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	db, ok := c.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("no connection")
	}

	serverID, _ := c.GetServerID(instanceName)

	var snap models.PgMemoryStatsSnapshot
	snap.ServerID = serverID
	snap.Timestamp = time.Now().UTC()

	// Exclude system databases to get accurate user-workload stats.
	q := `SELECT /* SQL_OPTIMA */
		COALESCE(sum(blks_hit), 0),
		COALESCE(sum(blks_read), 0),
		COALESCE(sum(temp_files), 0),
		COALESCE(sum(temp_bytes), 0)
	FROM pg_stat_database
	WHERE datname NOT IN ('template0', 'template1', 'postgres')
	  AND datname IS NOT NULL`
	_ = db.QueryRowContext(ctx, q).Scan(&snap.BlksHit, &snap.BlksRead, &snap.TempFiles, &snap.TempBytes)

	// BGWriter stats — shows checkpoint vs backend buffer write pressure.
	q2 := `SELECT /* SQL_OPTIMA */
		COALESCE(buffers_checkpoint, 0),
		COALESCE(buffers_clean, 0),
		COALESCE(buffers_backend, 0)
	FROM pg_stat_bgwriter`
	_ = db.QueryRowContext(ctx, q2).Scan(&snap.BuffersCheckpoint, &snap.BuffersClean, &snap.BuffersBackend)

	// Connection counts from pg_stat_activity.
	active, waiting, _ := c.FetchActiveWaitingSessions(ctx, instanceName)
	idle, _, _ := c.FetchIdleSessions(ctx, instanceName)
	snap.ActiveConnections = active
	snap.IdleConnections = idle
	snap.TotalConnections = active + idle + waiting

	return &snap, nil
}

func (c *PgRepository) CollectPgMemoryComponents(ctx context.Context, instanceName string) (*models.PgMemoryComponentsSnapshot, error) {
	db, ok := c.GetConn(instanceName)
	if !ok {
		return nil, fmt.Errorf("no connection")
	}

	serverID, _ := c.GetServerID(instanceName)

	var snap models.PgMemoryComponentsSnapshot
	snap.ServerID = serverID
	snap.Timestamp = time.Now().UTC()

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()

	// Fetch name, setting (numeric value), and unit so we can convert to MB accurately.
	// PostgreSQL unit examples: "8kB" (shared_buffers), "kB" (work_mem), "MB", "GB".
	rows, err := db.QueryContext(ctx, `SELECT /* SQL_OPTIMA */
		name, setting, COALESCE(unit, '')
	FROM pg_settings
	WHERE name IN (
		'shared_buffers', 'work_mem', 'maintenance_work_mem',
		'wal_buffers', 'temp_buffers', 'effective_cache_size', 'max_connections'
	)`)
	if err != nil {
		return &snap, nil
	}
	defer rows.Close()

	for rows.Next() {
		var name, setting, unit string
		if err := rows.Scan(&name, &setting, &unit); err != nil {
			continue
		}
		switch name {
		case "shared_buffers":
			snap.SharedBuffersMB = pgSettingToMB(setting, unit)
		case "work_mem":
			snap.WorkMemMB = pgSettingToMB(setting, unit)
		case "maintenance_work_mem":
			snap.MaintenanceWorkMemMB = pgSettingToMB(setting, unit)
		case "wal_buffers":
			snap.WalBuffersMB = pgSettingToMB(setting, unit)
		case "temp_buffers":
			snap.TempBuffersMB = pgSettingToMB(setting, unit)
		case "effective_cache_size":
			snap.EffectiveCacheSizeMB = pgSettingToMB(setting, unit)
		case "max_connections":
			v, _ := strconv.Atoi(setting)
			snap.MaxConnections = v
		}
	}

	return &snap, nil
}

// pgSettingToMB converts a pg_settings value + unit pair to megabytes.
// PostgreSQL reports memory GUC values as an integer multiplied by the unit:
//   - "8kB"  → 8-kilobyte pages (shared_buffers, wal_buffers, temp_buffers, effective_cache_size)
//   - "kB"   → kilobytes (work_mem, maintenance_work_mem)
//   - "MB"   → megabytes
//   - "GB"   → gigabytes
func pgSettingToMB(setting, unit string) int64 {
	v, err := strconv.ParseFloat(setting, 64)
	if err != nil || v <= 0 {
		return 0
	}
	switch strings.ToUpper(unit) {
	case "8KB":
		return int64(v * 8 / 1024) // 8kB pages → MB
	case "KB":
		return int64(v / 1024) // kB → MB
	case "B":
		return int64(v / 1048576) // bytes → MB
	case "MB":
		return int64(v)
	case "GB":
		return int64(v * 1024)
	default:
		return 0
	}
}
