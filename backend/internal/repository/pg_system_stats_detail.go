// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Detailed system statistics for PostgreSQL server resources.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
)

type PgSystemStatsDetail struct {
	CPUUsagePct         float64 `json:"cpu_usage_pct"`
	MemoryUsedPct       float64 `json:"memory_used_pct"`
	TotalMemoryBytes    int64   `json:"total_memory_bytes"`
	AvailableMemoryBytes int64  `json:"available_memory_bytes"`
	SharedBuffersBytes  int64   `json:"shared_buffers_bytes"`
}

// GetSystemStatsDetail tries to read host-level memory from pg_os_info() and CPU util from pg_stat_cpu().
// If those helper views/functions are not available, it falls back to existing approximations.
func (c *PgRepository) GetSystemStatsDetail(instanceName string) (*PgSystemStatsDetail, error) {
	c.mutex.RLock()
	db, ok := c.conns[instanceName]
	c.mutex.RUnlock()
	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	out := &PgSystemStatsDetail{}

	// shared_buffers bytes (always available)
	_ = db.QueryRow(`
		SELECT (setting::bigint * 8192)
		FROM pg_settings
		WHERE name = 'shared_buffers'
	`).Scan(&out.SharedBuffersBytes)

	// Prefer pg_os_info + pg_stat_cpu if available.
	err := db.QueryRow(`
		SELECT 
			COALESCE((SELECT util FROM pg_stat_cpu() WHERE util IS NOT NULL LIMIT 1), 0) AS cpu_usage,
			COALESCE(100 * (1 - (available_memory::float / NULLIF(total_memory::float,0))), 0) AS mem_used_pct,
			COALESCE(total_memory, 0) AS total_mem,
			COALESCE(available_memory, 0) AS avail_mem
		FROM pg_os_info()
		LIMIT 1
	`).Scan(&out.CPUUsagePct, &out.MemoryUsedPct, &out.TotalMemoryBytes, &out.AvailableMemoryBytes)
	if err == nil && out.TotalMemoryBytes > 0 {
		return out, nil
	}

	// Fallback to existing lightweight approximations.
	cpu, mem, err2 := c.GetSystemStats(instanceName)
	if err2 != nil {
		return out, err2
	}
	out.CPUUsagePct = cpu
	out.MemoryUsedPct = mem
	return out, nil
}

