/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Dashboard v2 repository for real-time triage data.
 * Metadata:
 *   - Feature: SQL Server Health Dashboard V2
 *   - Layer: Repository
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
package repository

import (
	"log/slog"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
)

// FetchHealthV2 gathers all metrics for the SQL Server Health Dashboard V2.
func (c *SqlServerRepository) FetchHealthV2(ctx context.Context, instanceName string) (models.HealthV2DashboardResponse, error) {
	var res models.HealthV2DashboardResponse
	res.InstanceName = instanceName
	res.LastUpdate = time.Now().UTC()

	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return res, fmt.Errorf("instance %s not found or connection is nil", instanceName)
	}

	// 1. KPIs
	kpis, _, err := c.getKPIsV2(ctx, db)
	if err != nil {
		slog.Error("[Repository] Error fetching Health V2 KPIs", "err", err)
	}
	res.KPIs = kpis

	// 2. TempDB Health
	tempdb, err := c.getTempDBHealthV2(ctx, db)
	if err != nil {
		slog.Error("[Repository] Error fetching Health V2 TempDB", "err", err)
	}
	res.TempDB = tempdb

	// 3. Problems (Active Queries & Blocking)
	longRunning, _ := CollectLongRunningQueries(ctx, db)
	blocking, _ := CollectBlockingLocks(ctx, db)

	if longRunning == nil {
		longRunning = []models.LongRunningQuery{}
	}
	if blocking == nil {
		blocking = []models.BlockingNode{}
	}

	res.Problems.LongRunning = longRunning
	res.Problems.Blocking = blocking

	return res, nil
}

// FetchHealthV2KPIs gathers the real-time triage KPIs and the SQL Server start time.
func (c *SqlServerRepository) FetchHealthV2KPIs(ctx context.Context, instanceName string) (models.HealthV2KPIs, time.Time, error) {
	var k models.HealthV2KPIs
	var startTime time.Time

	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return k, startTime, fmt.Errorf("instance %s not found", instanceName)
	}

	return c.getKPIsV2(ctx, db)
}

func (c *SqlServerRepository) getKPIsV2(ctx context.Context, db *sql.DB) (models.HealthV2KPIs, time.Time, error) {
	var k models.HealthV2KPIs
	var startTime time.Time

	instanceName := ""
	// Identify instance name for cache lookup
	c.mutex.RLock()
	for name, conn := range c.conns {
		if conn == db {
			instanceName = name
			break
		}
	}
	c.mutex.RUnlock()

	c.mutex.RLock()
	cached, hasCache := c.serverInfoCache[instanceName]
	c.mutex.RUnlock()

	var sqlPath string
	if hasCache {
		sqlPath = filepath.Join("infrastructure", "sql_scripts", "collection", "sqlserver_kpi_metrics_light_v2.sql")
	} else {
		sqlPath = filepath.Join("infrastructure", "sql_scripts", "collection", "sqlserver_kpi_metrics_v2.sql")
	}

	if _, err := os.Stat(sqlPath); os.IsNotExist(err) {
		sqlPath = filepath.Join("..", sqlPath)
	}

	query, err := os.ReadFile(sqlPath)
	if err != nil {
		return k, startTime, err
	}

	serverID, _ := c.GetServerID(instanceName)

	if hasCache {
		var pr, br, sc, ls float64
		ctx, cancel := WithQueryTimeout(ctx, 0)
		defer cancel()
		err = db.QueryRowContext(ctx, string(query)).Scan(
			&k.SqlCpuPct,
			&k.RunnableTasks,
			&k.MemGrantsPending,
			&pr,
			&k.LogWriteWaitMs,
			&br,
			&sc,
			&ls,
			&k.TargetServerMemoryMB,
			&k.TotalServerMemoryMB,
			&k.BlockedSessions,
			&k.UserConnections,
		)
		k.PageReadsPerSec, k.BatchRequests, k.Compilations, k.LoginsPerSec = c.applyDeltas(serverID, pr, br, sc, ls)

		k.Edition = cached.Edition
		startTime = cached.StartTime
		diff := time.Since(startTime)
		k.Uptime = fmt.Sprintf("%dd %dh %dm", int(diff.Hours()/24), int(diff.Hours())%24, int(diff.Minutes())%60)
	} else {
		var sqlCpu sql.NullFloat64
		var edition sql.NullString
		var pr, br, sc, ls float64

		ctx, cancel := WithQueryTimeout(ctx, 0)
		defer cancel()
		err = db.QueryRowContext(ctx, string(query)).Scan(
			&sqlCpu,
			&k.RunnableTasks,
			&k.MemGrantsPending,
			&pr,
			&k.LogWriteWaitMs,
			&br,
			&sc,
			&ls,
			&k.TargetServerMemoryMB,
			&k.TotalServerMemoryMB,
			&k.BlockedSessions,
			&k.UserConnections,
			&edition,
			&startTime,
		)
		if err == nil {
			k.SqlCpuPct = sqlCpu.Float64
			k.PageReadsPerSec, k.BatchRequests, k.Compilations, k.LoginsPerSec = c.applyDeltas(serverID, pr, br, sc, ls)

			k.Edition = "Unknown"
			if edition.Valid {
				k.Edition = edition.String
			}
			diff := time.Since(startTime)
			k.Uptime = fmt.Sprintf("%dd %dh %dm", int(diff.Hours()/24), int(diff.Hours())%24, int(diff.Minutes())%60)

			// Update cache
			c.mutex.Lock()
			c.serverInfoCache[instanceName] = CachedServerInfo{Edition: k.Edition, StartTime: startTime}
			c.mutex.Unlock()
		}
	}

	if err != nil {
		return k, startTime, err
	}

	// Status Logic
	k.InstanceStatus = "Healthy"
	if k.SqlCpuPct > 80 || k.RunnableTasks > 15 || k.MemGrantsPending > 5 || k.BlockedSessions > 5 {
		k.InstanceStatus = "Critical"
	} else if k.SqlCpuPct > 60 || k.RunnableTasks > 5 || k.MemGrantsPending > 0 || k.BlockedSessions > 0 {
		k.InstanceStatus = "Warning"
	}

	return k, startTime, nil
}

func (c *SqlServerRepository) applyDeltas(serverID uuid.UUID, pageReads, batchReq, compilations, logins float64) (float64, float64, float64, float64) {
	now := time.Now().UTC()
	current := map[string]int64{
		"Page Reads/sec":       int64(pageReads),
		"Batch Requests/sec":   int64(batchReq),
		"SQL Compilations/sec": int64(compilations),
		"Logins/sec":           int64(logins),
	}

	c.perfCounterMu.Lock()
	prev := c.perfCounterSnapshots[serverID]
	c.perfCounterSnapshots[serverID] = &perfCounterState{values: current, capturedAt: now}
	c.perfCounterMu.Unlock()

	if prev == nil || prev.capturedAt.IsZero() {
		return 0, 0, 0, 0
	}

	elapsed := now.Sub(prev.capturedAt).Seconds()
	if elapsed <= 0 {
		return 0, 0, 0, 0
	}

	delta := func(name string) float64 {
		d := float64(current[name] - prev.values[name])
		if d < 0 {
			return 0
		}
		return d / elapsed
	}

	return delta("Page Reads/sec"), delta("Batch Requests/sec"), delta("SQL Compilations/sec"), delta("Logins/sec")
}

func (c *SqlServerRepository) getTempDBHealthV2(ctx context.Context, db *sql.DB) (models.TempDBHealth, error) {
	var t models.TempDBHealth

	sqlPath := filepath.Join("infrastructure", "sql_scripts", "collection", "sqlserver_tempdb_health_v2.sql")
	if _, err := os.Stat(sqlPath); os.IsNotExist(err) {
		sqlPath = filepath.Join("..", sqlPath)
	}

	query, err := os.ReadFile(sqlPath)
	if err != nil {
		return t, err
	}

	var userObj, internalObj, versionStore, free, logUsed sql.NullFloat64
	var contention sql.NullBool
	ctx, cancel := WithQueryTimeout(ctx, 0)
	defer cancel()
	err = db.QueryRowContext(ctx, string(query)).Scan(
		&userObj,
		&internalObj,
		&versionStore,
		&free,
		&logUsed,
		&contention,
	)
	t.UserObjMB = userObj.Float64
	t.InternalObjMB = internalObj.Float64
	t.VersionStoreMB = versionStore.Float64
	t.FreeMB = free.Float64
	t.LogUsedMB = logUsed.Float64
	t.ContentionFound = contention.Bool

	return t, err
}
