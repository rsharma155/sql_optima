// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: SQL Server dashboard telemetry (STRICT STAGING ONLY).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"log/slog"
	"context"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

func (c *SqlServerRepository) FetchLiveTelemetry(ctx context.Context, instanceName string, prev models.DashboardMetrics) models.DashboardMetrics {
	var metrics models.DashboardMetrics
	metrics.InstanceName = instanceName
	metrics.MemHistory = prev.MemHistory
	metrics.CPUHistory = prev.CPUHistory

	if c.LocalPool == nil {
		slog.Error("[SQLSERVER] datasource error for %s: TimescaleDB not connected", "val", instanceName)
		return metrics
	}

	serverID, ok := c.GetServerID(instanceName)
	if !ok {
		slog.Info("[SQLSERVER] instance %s not found in registry", "val", instanceName)
		return metrics
	}

	// 1. Memory from Staging
	mQuery := `
		SELECT /* SQL_OPTIMA STAGING ONLY */ 
			ISNULL(100.0 * (CAST(SUM(CASE WHEN metric_name = 'Total Physical Memory' THEN metric_value END) AS FLOAT) - 
			CAST(SUM(CASE WHEN metric_name = 'Available Physical Memory' THEN metric_value END) AS FLOAT)) / 
			NULLIF(CAST(SUM(CASE WHEN metric_name = 'Total Physical Memory' THEN metric_value END) AS FLOAT), 0), 0)
		FROM staging.sqlserver_perf_system_raw 
		WHERE server_id = $1 AND category = 'SYS_MEM'
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.sqlserver_perf_system_raw WHERE server_id = $1)`

	_ = c.LocalPool.QueryRow(context.Background(), mQuery, serverID).Scan(&metrics.MemoryUsage)

	// 2. Active Users from Staging
	sessionQuery := `
		SELECT /* SQL_OPTIMA STAGING ONLY */ 
			COUNT(*) FILTER (WHERE session_status = 'running' AND login_name NOT IN ('dbmonitor_user', 'sql-optima'))
		FROM staging.sqlserver_session_request_raw
		WHERE server_id = $1
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.sqlserver_session_request_raw WHERE server_id = $1)`
	_ = c.LocalPool.QueryRow(context.Background(), sessionQuery, serverID).Scan(&metrics.ActiveUsers)

	// 3. Snapshot from Staging
	if snapshots, err := c.CollectSessionSnapshotStaging(ctx, instanceName); err == nil {
		metrics.SessionSnapshots = snapshots
	}

	return metrics
}

func (c *SqlServerRepository) FetchHistoricalTelemetry(instanceName string, prev models.DashboardMetrics) models.DashboardMetrics {
	var metrics models.DashboardMetrics
	metrics.InstanceName = instanceName
	metrics.WaitHistory = prev.WaitHistory
	metrics.FileHistory = prev.FileHistory

	if c.LocalPool == nil {
		return metrics
	}

	serverID, ok := c.GetServerID(instanceName)
	if !ok {
		return metrics
	}

	var wSnap models.WaitSnapshot
	wSnap.Timestamp = time.Now().Format("2006-01-02 15:04:05")

	var fSnap models.FileIOSnapshot
	fSnap.Timestamp = time.Now().Format("2006-01-02 15:04:05")

	// 1. Wait Stats from Staging
	wQuery := `
		SELECT /* SQL_OPTIMA STAGING ONLY */ metric_name, metric_value 
		FROM staging.sqlserver_perf_system_raw 
		WHERE server_id = $1 AND category = 'WAIT'
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.sqlserver_perf_system_raw WHERE server_id = $1)`

	wRows, err := c.LocalPool.Query(context.Background(), wQuery, serverID)
	if err == nil && wRows != nil {
		defer wRows.Close()
		for wRows.Next() {
			var wt string
			var wtVal int64
			if err := wRows.Scan(&wt, &wtVal); err == nil {
				wtMs := float64(wtVal)
				if strings.HasPrefix(wt, "PAGEIOLATCH") {
					wSnap.DiskRead += wtMs
				} else if strings.HasPrefix(wt, "LCK_") {
					wSnap.Blocking += wtMs
				}
			}
		}
	}

	// 2. File IO from Staging
	fQuery := `
		SELECT /* SQL_OPTIMA STAGING ONLY */ db_name, physical_name, type_desc, num_of_reads, num_of_writes, io_stall_read_ms, io_stall_write_ms 
		FROM staging.sqlserver_io_raw 
		WHERE server_id = $1 
		  AND capture_timestamp = (SELECT max(capture_timestamp) FROM staging.sqlserver_io_raw WHERE server_id = $1)`

	fRows, err := c.LocalPool.Query(context.Background(), fQuery, serverID)
	if err == nil && fRows != nil {
		defer fRows.Close()
		for fRows.Next() {
			var f models.FileIOStat
			if err := fRows.Scan(&f.DatabaseName, &f.PhysicalName, &f.FileType, &f.NumOfReads, &f.NumOfWrites, &f.IoStallReadMs, &f.IoStallWriteMs); err == nil {
				fSnap.Files = append(fSnap.Files, f)
			}
		}
	}

	metrics.WaitHistory = append(metrics.WaitHistory, wSnap)
	metrics.FileHistory = append(metrics.FileHistory, fSnap)
	return metrics
}
