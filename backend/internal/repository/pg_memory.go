// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL memory metrics collection repository.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// CollectPgMemoryStats fetches PostgreSQL internal memory usage metrics.
func (c *PgRepository) CollectPgMemoryStats(ctx context.Context, instanceName string) (*models.PgMemoryStatsSnapshot, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	snap := &models.PgMemoryStatsSnapshot{
		Timestamp:    time.Now().UTC(),
		InstanceName: instanceName,
	}

	// 1. Connections
	connQuery := `
			/* SQL_OPTIMA */ SELECT 
			COUNT(*) FILTER (WHERE state = 'active') AS active_connections,
			COUNT(*) FILTER (WHERE state = 'idle')   AS idle_connections,
			COUNT(*) AS total_connections
		FROM pg_stat_activity`
	if err := db.QueryRowContext(ctx, connQuery).Scan(&snap.ActiveConnections, &snap.IdleConnections, &snap.TotalConnections); err != nil {
		log.Printf("[POSTGRES] CollectPgMemoryStats connections error: %v", err)
	}

	// 2. Cache Stats & Temp Spill Stats
	dbQuery := `
		/* SQL_OPTIMA */ SELECT  
			SUM(blks_hit)  AS blks_hit,
			SUM(blks_read) AS blks_read,
			SUM(temp_files) AS temp_files,
			SUM(temp_bytes) AS temp_bytes
		FROM pg_stat_database`
	if err := db.QueryRowContext(ctx, dbQuery).Scan(&snap.BlksHit, &snap.BlksRead, &snap.TempFiles, &snap.TempBytes); err != nil {
		log.Printf("[POSTGRES] CollectPgMemoryStats database stats error: %v", err)
	}

	// 3. BGWriter
	bgQuery := `
		/* SQL_OPTIMA */ SELECT  
			buffers_checkpoint,
			buffers_clean,
			buffers_backend
		FROM pg_stat_bgwriter`
	if err := db.QueryRowContext(ctx, bgQuery).Scan(&snap.BuffersCheckpoint, &snap.BuffersClean, &snap.BuffersBackend); err != nil {
		log.Printf("[POSTGRES] CollectPgMemoryStats bgwriter error: %v", err)
	}

	// 4. Postgres Process Memory (Linux only, co-located)
	if runtime.GOOS == "linux" {
		pgRaw := c.runBash("ps -eo rss,vsz,comm 2>/dev/null | grep postgres | awk '{rss+=$1; vsz+=$2} END {print rss \",\" vsz}'")
		parts := strings.Split(pgRaw, ",")
		if len(parts) == 2 {
			rss, _ := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			vsz, _ := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			snap.PostgresRSSMB = rss / 1024 // KB to MB
			snap.PostgresVSZMB = vsz / 1024
		}
	} else {
		// Fallback/estimate for non-linux or non-colocated
		var sharedBuffers int64
		if err := db.QueryRowContext(ctx, "SELECT /* SQL_OPTIMA */   setting::bigint * 8 / 1024 FROM pg_settings WHERE name = 'shared_buffers'").Scan(&sharedBuffers); err != nil {
			log.Printf("[POSTGRES] CollectPgMemoryStats shared_buffers scan error: %v", err)
		}
		snap.PostgresRSSMB = int64(float64(sharedBuffers) * 1.2) // very rough estimate
	}

	return snap, nil
}

// CollectPgMemoryComponents fetches PostgreSQL memory configuration settings.
func (c *PgRepository) CollectPgMemoryComponents(ctx context.Context, instanceName string) (*models.PgMemoryComponentsSnapshot, error) {
	db, ok := c.GetConn(instanceName)
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found for %s", instanceName)
	}

	snap := &models.PgMemoryComponentsSnapshot{
		Timestamp:    time.Now().UTC(),
		InstanceName: instanceName,
	}

	query := `
		SELECT  
			(SELECT    setting::bigint * 8 / 1024 FROM pg_settings WHERE name = 'shared_buffers') AS shared_buffers_mb,
			(SELECT    setting::bigint / 1024 FROM pg_settings WHERE name = 'work_mem') AS work_mem_mb,
			(SELECT    setting::bigint / 1024 FROM pg_settings WHERE name = 'maintenance_work_mem') AS maintenance_work_mem_mb,
			(SELECT    setting::bigint * 8 / 1024 FROM pg_settings WHERE name = 'wal_buffers') AS wal_buffers_mb,
			(SELECT    setting::bigint * 8 / 1024 FROM pg_settings WHERE name = 'temp_buffers') AS temp_buffers_mb,
			(SELECT    setting::int FROM pg_settings WHERE name = 'max_connections') AS max_connections`

	err := db.QueryRowContext(ctx, query).Scan(
		&snap.SharedBuffersMB, &snap.WorkMemMB, &snap.MaintenanceWorkMemMB, &snap.WalBuffersMB, &snap.TempBuffersMB, &snap.MaxConnections,
	)
	if err != nil {
		return nil, err
	}

	return snap, nil
}

// CollectPgHostMemory fetches OS-level memory metrics (Linux only).
func (c *PgRepository) CollectPgHostMemory(instanceName string) (*models.PgHostMemorySnapshot, error) {
	snap := &models.PgHostMemorySnapshot{
		Timestamp:    time.Now().UTC(),
		InstanceName: instanceName,
		ServerID:     instanceName,
	}

	if runtime.GOOS != "linux" {
		// Mock data for development on Darwin/Windows
		snap.TotalMemoryMB = 16384
		snap.UsedMemoryMB = 8192
		snap.FreeMemoryMB = 4096
		snap.CachedMemoryMB = 4096
		return snap, nil
	}

	// 1. Memory Info from /proc/meminfo
	memInfo := c.runBash("cat /proc/meminfo")
	snap.TotalMemoryMB = c.parseMemInfo(memInfo, "MemTotal:") / 1024
	snap.FreeMemoryMB = c.parseMemInfo(memInfo, "MemFree:") / 1024
	snap.CachedMemoryMB = c.parseMemInfo(memInfo, "Cached:") / 1024
	snap.BufferedMemoryMB = c.parseMemInfo(memInfo, "Buffers:") / 1024
	snap.SwapTotalMB = c.parseMemInfo(memInfo, "SwapTotal:") / 1024
	snap.SwapUsedMB = snap.SwapTotalMB - (c.parseMemInfo(memInfo, "SwapFree:") / 1024)

	snap.UsedMemoryMB = snap.TotalMemoryMB - snap.FreeMemoryMB - snap.CachedMemoryMB - snap.BufferedMemoryMB

	// 2. Page Faults from /proc/vmstat
	vmStat := c.runBash("cat /proc/vmstat")
	snap.PageFaults = c.parseMemInfo(vmStat, "pgfault")
	snap.MajorPageFaults = c.parseMemInfo(vmStat, "pgmajfault")

	return snap, nil
}

func (c *PgRepository) runBash(script string) string {
	out, err := exec.Command("bash", "-c", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (c *PgRepository) parseMemInfo(content, key string) int64 {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, key) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				val, _ := strconv.ParseInt(fields[1], 10, 64)
				return val
			}
		}
	}
	return 0
}
