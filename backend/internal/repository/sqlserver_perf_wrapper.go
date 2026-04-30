// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server performance metrics wrapper functions for service layer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
)

func (c *SqlServerRepository) FetchLatchStats(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectLatchStats(db)
}

func (c *SqlServerRepository) FetchWaitingTasks(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectWaitingTasks(db)
}

func (c *SqlServerRepository) FetchMemoryGrants(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectMemoryGrants(db)
}

func (c *SqlServerRepository) FetchProcedureStats(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectProcedureStats(db)
}

func (c *SqlServerRepository) FetchFileIOLatency(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectFileIOLatency(db)
}

func (c *SqlServerRepository) FetchSpinlockStats(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectSpinlockStats(db)
}

func (c *SqlServerRepository) FetchMemoryClerks(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectMemoryClerks(db)
}

func (c *SqlServerRepository) FetchTempdbStats(instanceName string) ([]map[string]interface{}, error) {
	db, _ := c.GetConn(instanceName)
	if db == nil {
		return nil, fmt.Errorf("connection not found")
	}
	return c.CollectTempDBStats(db)
}

func (c *SqlServerRepository) FetchSchedulerWG(instanceName string) ([]map[string]interface{}, error) {
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

	rows, err := db.Query(query)
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
