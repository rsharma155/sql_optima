// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server performance metrics wrapper functions for service layer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"fmt"
)

func (c *SqlServerRepository) FetchLatchStats(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectLatchStats(ctx, db)
}

func (c *SqlServerRepository) FetchWaitingTasks(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectWaitingTasks(ctx, db)
}

func (c *SqlServerRepository) FetchMemoryGrants(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectMemoryGrants(ctx, db)
}

func (c *SqlServerRepository) FetchProcedureStats(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectProcedureStats(ctx, db)
}

func (c *SqlServerRepository) FetchFileIOLatency(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectFileIOLatency(ctx, db)
}

func (c *SqlServerRepository) FetchSpinlockStats(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectSpinlockStats(ctx, db)
}

func (c *SqlServerRepository) FetchMemoryClerks(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectMemoryClerks(ctx, db)
}

func (c *SqlServerRepository) FetchTempdbStats(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectTempDBStats(ctx, db)
}

func (c *SqlServerRepository) FetchSchedulerWG(ctx context.Context, instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT   
			COALESCE(rp.name, 'default') AS pool_name,
			COALESCE(wg.name, 'default') AS group_name,
			COALESCE(wgs.active_request_count, 0) AS active_requests,
			COALESCE(wgs.queued_request_count, 0) AS queued_requests,
			COALESCE(wgs.total_cpu_usage_ms * 100.0 / NULLIF(rgs.total_cpu_usage_ms, 0), 0) AS cpu_usage_percent
		FROM sys.dm_resource_governor_workload_groups wg
		LEFT JOIN sys.resource_governor_workload_groups wgm ON wg.group_id = wgm.group_id
		LEFT JOIN sys.dm_resource_governor_resource_pools rp ON wgm.pool_id = rp.pool_id
		LEFT JOIN sys.dm_resource_governor_workload_groups_stats wgs ON wg.group_id = wgs.group_id
		CROSS JOIN (SELECT /* SQL_OPTIMA */   SUM(total_cpu_usage_ms) AS total_cpu_usage_ms FROM sys.dm_resource_governor_workload_groups_stats) rgs
		ORDER BY cpu_usage_percent DESC, pool_name, group_name
	`

	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []map[string]interface{}
	for rows.Next() {
		var poolName, groupName string
		var active, queued int64
		var cpuPct float64
		if err := rows.Scan(&poolName, &groupName, &active, &queued, &cpuPct); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"pool_name":         poolName,
			"group_name":        groupName,
			"active_requests":   active,
			"queued_requests":   queued,
			"cpu_usage_percent": cpuPct,
		})
	}
	return results, nil
}
