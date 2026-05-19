// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server global metrics (CPU, memory) from system DMVs.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"github.com/rsharma155/sql_optima/internal/models"
)

func (c *SqlServerRepository) GetGlobalMetric(ctx context.Context, name string, base models.GlobalInstanceMetric) models.GlobalInstanceMetric {
	c.mutex.RLock()
	db, ok := c.conns[name]
	c.mutex.RUnlock()

	if !ok || db == nil {
		base.Status = 2
		base.Error = "Connection Context Lost"
		return base
	}

	if err := db.Ping(); err != nil {
		base.Status = 2
		base.Error = err.Error()
		return base
	}

	base.Status = 0

	cpuQuery := `
		/* SQL_OPTIMA */	
		SELECT  TOP 1 
			record.value('(./Record/SchedulerMonitorEvent/SystemHealth/ProcessUtilization)[1]', 'int') AS [SQLProcessUtilization]
		FROM (
			SELECT /* SQL_OPTIMA */   [timestamp], CONVERT(xml, record) AS [record]
			FROM sys.dm_os_ring_buffers
			WHERE ring_buffer_type = N'RING_BUFFER_SCHEDULER_MONITOR'
			AND record LIKE '%<SystemHealth>%'
		) AS x ORDER BY [timestamp] DESC
	`
	var cpu int
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	if err := db.QueryRowContext(ctx, cpuQuery).Scan(&cpu); err == nil {
		base.CPUUsage = float64(cpu)
	}

	base.MemoryPct = 0

	return base
}
