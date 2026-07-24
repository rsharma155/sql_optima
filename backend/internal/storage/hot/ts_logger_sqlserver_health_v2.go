/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Dashboard v2 KPI storage implementation.
 * Metadata:
 *   - Feature: SQL Server Health Dashboard V2
 *   - Layer: Infrastructure (Storage)
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

package hot

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// LogSqlServerHealthV2KPIs persists real-time triage KPIs to TimescaleDB.
func (tl *TimescaleLogger) LogSqlServerHealthV2KPIs(ctx context.Context, serverID uuid.UUID, k models.HealthV2KPIs, uptimeSeconds int64) error {
	if tl.pool == nil {
		return fmt.Errorf("TimescaleDB not connected")
	}

	query := `
		INSERT INTO sqlserver_health_kpis_v2 (
			capture_timestamp, server_id, sql_cpu_pct, runnable_tasks, 
			mem_grants_pending, page_reads_per_sec, log_write_wait_ms, batch_requests, 
			compilations, logins_per_sec, target_server_memory_mb, total_server_memory_mb,
			blocked_sessions, user_connections, max_connections, instance_status, 
			edition, uptime_seconds
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
	`
	_, err := tl.pool.Exec(ctx, query,
		time.Now().UTC(),
		serverID,
		k.SqlCpuPct,
		k.RunnableTasks,
		k.MemGrantsPending,
		k.PageReadsPerSec,
		k.LogWriteWaitMs,
		k.BatchRequests,
		k.Compilations,
		k.LoginsPerSec,
		k.TargetServerMemoryMB,
		k.TotalServerMemoryMB,
		k.BlockedSessions,
		k.UserConnections,
		k.MaxConnections,
		k.InstanceStatus,
		k.Edition,
		uptimeSeconds,
	)
	return err
}

// GetLatestHealthV2KPIs retrieves the most recent KPI snapshot from TimescaleDB.
func (tl *TimescaleLogger) GetLatestHealthV2KPIs(ctx context.Context, serverID uuid.UUID) (models.HealthV2KPIs, int64, error) {
	var k models.HealthV2KPIs
	var uptimeSeconds int64
	if tl.pool == nil {
		return k, 0, fmt.Errorf("TimescaleDB not connected")
	}

	query := `
		SELECT 
			sql_cpu_pct, runnable_tasks, mem_grants_pending, page_reads_per_sec, 
			log_write_wait_ms, batch_requests, compilations, logins_per_sec,
			target_server_memory_mb, total_server_memory_mb,
			blocked_sessions, user_connections, COALESCE(max_connections, 0), instance_status, edition, uptime_seconds
		FROM sqlserver_health_kpis_v2
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	err := tl.pool.QueryRow(ctx, query, serverID).Scan(
		&k.SqlCpuPct,
		&k.RunnableTasks,
		&k.MemGrantsPending,
		&k.PageReadsPerSec,
		&k.LogWriteWaitMs,
		&k.BatchRequests,
		&k.Compilations,
		&k.LoginsPerSec,
		&k.TargetServerMemoryMB,
		&k.TotalServerMemoryMB,
		&k.BlockedSessions,
		&k.UserConnections,
		&k.MaxConnections,
		&k.InstanceStatus,
		&k.Edition,
		&uptimeSeconds,
	)
	return k, uptimeSeconds, err
}
