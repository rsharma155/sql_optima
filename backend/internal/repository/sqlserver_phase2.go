// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Phase 2 dashboard metrics for advanced SQL Server monitoring.
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
)

// FetchBlockingSessionsCount returns number of currently blocked requests.
func (c *SqlServerRepository) FetchBlockingSessionsCount(ctx context.Context, instanceName string) (int, error) {
	db := c.conns[instanceName]
	if db == nil {
		return 0, fmt.Errorf("no connection for instance %s", instanceName)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var n int
	err := db.QueryRowContext(ctx, `
		/* SQL_OPTIMA */ 
		SELECT COUNT(*) AS blocking_sessions
		FROM sys.dm_exec_requests
		WHERE blocking_session_id <> 0;
	`).Scan(&n)
	return n, err
}

// FetchMemoryGrantsPending returns number of pending memory grants.
func (c *SqlServerRepository) FetchMemoryGrantsPending(ctx context.Context, instanceName string) (int, error) {
	db := c.conns[instanceName]
	if db == nil {
		return 0, fmt.Errorf("no connection for instance %s", instanceName)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var n int
	err := db.QueryRowContext(ctx, `
		/* SQL_OPTIMA */ SELECT   COUNT(*) AS memory_grants_pending
		FROM sys.dm_exec_query_memory_grants
		WHERE grant_time IS NULL;
	`).Scan(&n)
	return n, err
}

// FetchMemoryGrantsSummary returns a summary of current memory grant activity.
func (c *SqlServerRepository) FetchMemoryGrantsSummary(ctx context.Context, instanceName string) (map[string]interface{}, error) {
	db := c.conns[instanceName]
	if db == nil {
		return nil, fmt.Errorf("no connection for instance %s", instanceName)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var pending, active int
	var grantedMB float64
	// sys.dm_exec_query_memory_grants can be empty, so use COALESCE/ISNULL for SUM.
	err := db.QueryRowContext(ctx, `
		/* SQL_OPTIMA */ SELECT  
			ISNULL(SUM(CASE WHEN grant_time IS NULL THEN 1 ELSE 0 END), 0) AS pending_grants,
			ISNULL(SUM(CASE WHEN grant_time IS NOT NULL THEN 1 ELSE 0 END), 0) AS active_grants,
			ISNULL(SUM(granted_memory_kb), 0) / 1024.0 AS granted_memory_mb
		FROM sys.dm_exec_query_memory_grants;
	`).Scan(&pending, &active, &grantedMB)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"pending_grants":    pending,
		"active_grants":     active,
		"granted_memory_mb": grantedMB,
	}, nil
}

type PerfCounterSample struct {
	CounterName  string
	InstanceName sql.NullString
	Value        float64
}

// FetchPerfCounters fetches specific perf counters from sys.dm_os_performance_counters.
func (c *SqlServerRepository) FetchPerfCounters(ctx context.Context, instanceName string, counterNames []string) (map[string]PerfCounterSample, error) {
	db := c.conns[instanceName]
	if db == nil {
		return nil, fmt.Errorf("no connection for instance %s", instanceName)
	}
	if len(counterNames) == 0 {
		return map[string]PerfCounterSample{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	// Build IN list safely with parameters
	args := make([]any, 0, len(counterNames))
	placeholders := ""
	for i, n := range counterNames {
		if i > 0 {
			placeholders += ","
		}
		placeholders += fmt.Sprintf("@p%d", i+1)
		args = append(args, n)
	}

	q := fmt.Sprintf(`
		/* SQL_OPTIMA */ SELECT   RTRIM(counter_name) as counter_name, instance_name, CAST(cntr_value AS FLOAT) AS cntr_value
		FROM sys.dm_os_performance_counters
		WHERE RTRIM(counter_name) IN (%s);
	`, placeholders)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]PerfCounterSample)
	for rows.Next() {
		var s PerfCounterSample
		if err := rows.Scan(&s.CounterName, &s.InstanceName, &s.Value); err != nil {
			return nil, err
		}
		// Keep the first sample per counter_name (many counters have instance variants).
		if _, exists := out[s.CounterName]; !exists {
			out[s.CounterName] = s
		}
	}
	return out, rows.Err()
}

// FetchWaitStatsCumulative returns cumulative wait_time_ms per wait_type (filtered).
func (c *SqlServerRepository) FetchWaitStatsCumulative(ctx context.Context, instanceName string) (map[string]float64, error) {
	db := c.conns[instanceName]
	if db == nil {
		return nil, fmt.Errorf("no connection for instance %s", instanceName)
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		/* SQL_OPTIMA */ SELECT   wait_type, CAST(wait_time_ms AS FLOAT) AS wait_time_ms
		FROM sys.dm_os_wait_stats
		WHERE wait_type NOT LIKE '%SLEEP%';
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]float64)
	for rows.Next() {
		var wt string
		var ms float64
		if err := rows.Scan(&wt, &ms); err != nil {
			return nil, err
		}
		out[wt] = ms
	}
	return out, rows.Err()
}

// FetchTempdbUsagePercent estimates TempDB used% (user+internal objects) vs total.
func (c *SqlServerRepository) FetchTempdbUsagePercent(ctx context.Context, instanceName string) (float64, error) {
	db := c.conns[instanceName]
	if db == nil {
		return 0, fmt.Errorf("no connection for instance %s", instanceName)
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	// Use tempdb DMV without switching DB context.
	var usedMB, freeMB float64
	err := db.QueryRowContext(ctx, `
		/* SQL_OPTIMA */ SELECT  
			SUM(user_object_reserved_page_count + internal_object_reserved_page_count) * 8.0 / 1024.0 AS used_mb,
			SUM(unallocated_extent_page_count) * 8.0 / 1024.0 AS free_mb
		FROM tempdb.sys.dm_db_file_space_usage;
	`).Scan(&usedMB, &freeMB)
	if err != nil {
		return 0, err
	}
	total := usedMB + freeMB
	if total <= 0 {
		return 0, nil
	}
	return (usedMB / total) * 100.0, nil
}

type DBLogUsage struct {
	DatabaseName string
	UsedPercent  float64
	TotalMB      float64
	UsedMB       float64
}

// FetchMaxDBLogUsagePercent loops user DBs and returns the max used% sample.
// It is intentionally bounded and uses timeouts to avoid harming the instance.
func (c *SqlServerRepository) FetchMaxDBLogUsagePercent(ctx context.Context, instanceName string) (DBLogUsage, error) {
	db := c.conns[instanceName]
	if db == nil {
		return DBLogUsage{}, fmt.Errorf("no connection for instance %s", instanceName)
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	// Discover online user DBs.
	rows, err := db.QueryContext(ctx, `
		/* SQL_OPTIMA */ SELECT   name
		FROM sys.databases
		WHERE database_id > 4 AND state_desc = 'ONLINE';
	`)
	if err != nil {
		return DBLogUsage{}, err
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return DBLogUsage{}, err
		}
		dbs = append(dbs, n)
	}
	if err := rows.Err(); err != nil {
		return DBLogUsage{}, err
	}

	max := DBLogUsage{UsedPercent: -1}
	for _, name := range dbs {
		// Per-db context needed for sys.dm_db_log_space_usage.
		q := fmt.Sprintf(`
			/* SQL_OPTIMA */ 
			USE [%s];
			/* SQL_OPTIMA */ SELECT  
				total_log_size_mb,
				used_log_space_mb,
				used_log_space_in_percent
			FROM sys.dm_db_log_space_usage;
		`, name)

		var totalMB, usedMB, pct float64
		if err := db.QueryRowContext(ctx, q).Scan(&totalMB, &usedMB, &pct); err != nil {
			continue
		}
		if pct > max.UsedPercent {
			max = DBLogUsage{DatabaseName: name, UsedPercent: pct, TotalMB: totalMB, UsedMB: usedMB}
		}
	}

	if max.UsedPercent < 0 {
		return DBLogUsage{}, sql.ErrNoRows
	}
	return max, nil
}

// FetchFailedLoginsLast5Min returns a best-effort count of failed logins over the last 5 minutes.
// This uses RING_BUFFER_SECURITY_ERROR as a lightweight fallback (works in many environments).
// If unavailable (permissions / version), it returns 0 with error.
func (c *SqlServerRepository) FetchFailedLoginsLast5Min(ctx context.Context, instanceName string) (int, error) {
	db := c.conns[instanceName]
	if db == nil {
		return 0, fmt.Errorf("no connection for instance %s", instanceName)
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()

	// Convert ring buffer timestamp to wall-clock similarly to CPU ring buffer approach.
	// We count entries that mention "Login failed" within the last 5 minutes.
	var n int
	err := db.QueryRowContext(ctx, `
		/* SQL_OPTIMA */ 
		DECLARE @ts_now bigint = (SELECT  cpu_ticks/(cpu_ticks/ms_ticks) FROM sys.dm_os_sys_info WITH (NOLOCK)); 
		WITH rb AS (
			SELECT  
				DATEADD(ms, -1 * (@ts_now - [timestamp]), GETDATE()) AS event_time,
				CONVERT(nvarchar(max), record) AS rec
			FROM sys.dm_os_ring_buffers WITH (NOLOCK)
			WHERE ring_buffer_type = N'RING_BUFFER_SECURITY_ERROR'
		)
		SELECT   COUNT(*)
		FROM rb
		WHERE event_time >= DATEADD(minute, -5, GETDATE())
		  AND rec LIKE N'%Login failed%';
	`).Scan(&n)
	return n, err
}
