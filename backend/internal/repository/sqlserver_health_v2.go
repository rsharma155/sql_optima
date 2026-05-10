/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Dashboard v2 repository for real-time triage data.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	kpis, err := c.getKPIsV2(ctx, db)
	if err != nil {
		log.Printf("[Repository] Error fetching Health V2 KPIs: %v", err)
	}
	res.KPIs = kpis

	// 2. TempDB Health
	tempdb, err := c.getTempDBHealthV2(ctx, db)
	if err != nil {
		log.Printf("[Repository] Error fetching Health V2 TempDB: %v", err)
	}
	res.TempDB = tempdb

	// 3. Problems (Active Queries & Blocking)
	longRunning, _ := CollectLongRunningQueries(ctx, db)
	blocking, _ := CollectBlockingLocks(ctx, db)
	
	if longRunning == nil { longRunning = []models.LongRunningQuery{} }
	if blocking == nil { blocking = []models.BlockingNode{} }

	res.Problems.LongRunning = longRunning
	res.Problems.Blocking = blocking

	return res, nil
}

func (c *SqlServerRepository) getKPIsV2(ctx context.Context, db *sql.DB) (models.HealthV2KPIs, error) {
	var k models.HealthV2KPIs
	
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
		return k, err
	}

	if hasCache {
		err = db.QueryRowContext(ctx, string(query)).Scan(
			&k.SqlCpuPct,
			&k.RunnableTasks,
			&k.MemGrantsPending,
			&k.PageReadsPerSec,
			&k.LogWriteWaitMs,
			&k.BatchRequests,
			&k.Compilations,
			&k.BlockedSessions,
			&k.UserConnections,
		)
		k.Edition = cached.Edition
		startTime := cached.StartTime
		diff := time.Since(startTime)
		k.Uptime = fmt.Sprintf("%dd %dh %dm", int(diff.Hours()/24), int(diff.Hours())%24, int(diff.Minutes())%60)
	} else {
		var startTime time.Time
		var sqlCpu sql.NullFloat64
		var edition sql.NullString
		
		err = db.QueryRowContext(ctx, string(query)).Scan(
			&sqlCpu,
			&k.RunnableTasks,
			&k.MemGrantsPending,
			&k.PageReadsPerSec,
			&k.LogWriteWaitMs,
			&k.BatchRequests,
			&k.Compilations,
			&k.BlockedSessions,
			&k.UserConnections,
			&edition,
			&startTime,
		)
		if err == nil {
			k.SqlCpuPct = sqlCpu.Float64
			k.Edition = "Unknown"
			if edition.Valid { k.Edition = edition.String }
			diff := time.Since(startTime)
			k.Uptime = fmt.Sprintf("%dd %dh %dm", int(diff.Hours()/24), int(diff.Hours())%24, int(diff.Minutes())%60)
			
			// Update cache
			c.mutex.Lock()
			c.serverInfoCache[instanceName] = CachedServerInfo{Edition: k.Edition, StartTime: startTime}
			c.mutex.Unlock()
		}
	}
	
	if err != nil {
		return k, err
	}

	// Status Logic
	k.InstanceStatus = "Healthy"
	if k.SqlCpuPct > 80 || k.RunnableTasks > 15 || k.MemGrantsPending > 5 || k.BlockedSessions > 5 {
		k.InstanceStatus = "Critical"
	} else if k.SqlCpuPct > 60 || k.RunnableTasks > 5 || k.MemGrantsPending > 0 || k.BlockedSessions > 0 {
		k.InstanceStatus = "Warning"
	}

	return k, nil
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

	err = db.QueryRowContext(ctx, string(query)).Scan(
		&t.UserObjMB,
		&t.InternalObjMB,
		&t.VersionStoreMB,
		&t.FreeMB,
		&t.LogUsedMB,
		&t.ContentionFound,
	)
	
	return t, err
}
