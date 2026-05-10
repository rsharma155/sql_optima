// Package repository provides data access layer for database operations.
// It handles connections and queries for both PostgreSQL and SQL Server databases.
// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL system-level metrics, health alerts, and server information.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rsharma155/sql_optima/internal/models"
)

// GetServerInfo returns PostgreSQL version and uptime information.
// Uptime is calculated from pg_postmaster_start_time().
func (c *PgRepository) GetServerInfo(instanceName string) (version string, uptime string, err error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		log.Printf("[POSTGRES] GetServerInfo: connection not found for %s, attempting reconnect", instanceName)
		if c.reconnectInstance(instanceName) {
			c.mutex.RLock()
			db, ok = c.conns[strings.ToUpper(instanceName)]
			c.mutex.RUnlock()
			if !ok || db == nil {
				return "", "", fmt.Errorf("connection not found after reconnect")
			}
		} else {
			return "", "", fmt.Errorf("connection not found")
		}
	}

	// Get version string and extract version number
	err = db.QueryRow("SELECT /* SQL_OPTIMA */   version()").Scan(&version)
	if err != nil {
		return "", "", err
	}

	// Extract version number from full version string
	if strings.Contains(version, "PostgreSQL ") {
		parts := strings.Split(version, " ")
		if len(parts) > 1 {
			version = parts[1]
		}
	}

	// Get server start time and calculate uptime
	var startTime time.Time
	err = db.QueryRow("/* SQL_OPTIMA */ SELECT   pg_postmaster_start_time()").Scan(&startTime)
	if err != nil {
		return version, "", err
	}

	duration := time.Since(startTime)
	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	uptime = fmt.Sprintf("%d Days, %02d:%02d:%02d", days, hours, minutes, seconds)

	return version, uptime, nil
}

// GetSystemStats returns estimated CPU and memory usage metrics.
// Note: These are approximations based on PostgreSQL internals, not actual OS metrics.
func (c *PgRepository) GetSystemStats(instanceName string) (cpuUsage float64, memoryUsage float64, err error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return 0, 0, fmt.Errorf("connection not found")
	}

	// Estimate CPU based on active connections vs max connections
	var activeConnections int
	err = db.QueryRow("/* SQL_OPTIMA */ SELECT   count(*) FROM pg_stat_activity WHERE state = 'active'").Scan(&activeConnections)
	if err != nil {
		return 0, 0, err
	}

	var maxConnections int
	err = db.QueryRow("/* SQL_OPTIMA */ SELECT   setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&maxConnections)
	if err != nil {
		maxConnections = 100 // fallback
	}

	cpuUsage = float64(activeConnections) / float64(maxConnections) * 100
	if cpuUsage > 100 {
		cpuUsage = 100
	}

	// Estimate memory based on shared buffers
	var sharedBuffersMB int
	err = db.QueryRow(`
		SELECT /* SQL_OPTIMA */   (setting::bigint * 8192) / 1024 / 1024
		FROM pg_settings
		WHERE name = 'shared_buffers'
	`).Scan(&sharedBuffersMB)
	if err != nil {
		sharedBuffersMB = 128 // fallback
	}

	memoryUsage = float64(sharedBuffersMB) / 1024 * 100
	if memoryUsage > 100 {
		memoryUsage = 85 // cap at reasonable level
	}

	return cpuUsage, memoryUsage, nil
}

// GetConfig returns PostgreSQL configuration settings for key categories.
func (c *PgRepository) GetConfig(instanceName string) ([]models.PgConfigSetting, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	query := `
		/* SQL_OPTIMA */ SELECT  
			name,
			setting,
			unit,
			category,
			source,
			boot_val,
			reset_val
		FROM pg_settings
		WHERE category IN ('Autovacuum', 'Client Connection Defaults', 'Connections and Authentication', 'Resource Usage', 'Write-Ahead Log')
		ORDER BY category, name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []models.PgConfigSetting
	for rows.Next() {
		var s models.PgConfigSetting
		err := rows.Scan(&s.Name, &s.Value, &s.Unit, &s.Category, &s.Source, &s.BootVal, &s.ResetVal)
		if err != nil {
			continue
		}
		settings = append(settings, s)
	}

	return settings, nil
}

// GetAlerts returns potential issues and alerts based on current metrics.
func (c *PgRepository) GetAlerts(instanceName string) ([]models.PgAlert, error) {
	c.mutex.RLock()
	db, ok := c.conns[strings.ToUpper(instanceName)]
	c.mutex.RUnlock()

	if !ok || db == nil {
		return nil, fmt.Errorf("connection not found")
	}

	var alerts []models.PgAlert

	// Check for long-running idle-in-transaction queries
	longTxnQuery := `
		/* SQL_OPTIMA */ SELECT   
			pid,
			EXTRACT(EPOCH FROM (now() - xact_start)) / 60 as duration_minutes,
			query
		FROM pg_stat_activity
		WHERE xact_start IS NOT NULL
		AND state = 'idle in transaction'
		AND EXTRACT(EPOCH FROM (now() - xact_start)) > 900
		ORDER BY xact_start
		LIMIT 5
	`

	rows, err := db.Query(longTxnQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pid int
			var duration float64
			var query string
			if err := rows.Scan(&pid, &duration, &query); err != nil {
				log.Printf("[POSTGRES] Failed to scan long transaction row: %v", err)
				continue
			}
			alerts = append(alerts, models.PgAlert{
				Severity:   "CRITICAL",
				Metric:     fmt.Sprintf("Idle in Transaction (PID %d)", pid),
				Threshold:  "> 15 mins",
				CurrentVal: fmt.Sprintf("%.0f mins", duration),
				Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
				Status:     "ACTIVE",
			})
		}
	}

	// Check connection count threshold
	var connCount int
	if err := db.QueryRow("/* SQL_OPTIMA */ SELECT   count(*) FROM pg_stat_activity").Scan(&connCount); err != nil {
		log.Printf("[POSTGRES] Failed to query connection count: %v", err)
	}

	var maxConn int
	if err := db.QueryRow("/* SQL_OPTIMA */ SELECT   setting::int FROM pg_settings WHERE name = 'max_connections'").Scan(&maxConn); err != nil {
		log.Printf("[POSTGRES] Failed to query max_connections: %v", err)
	}

	if maxConn > 0 && float64(connCount)/float64(maxConn) > 0.8 {
		alerts = append(alerts, models.PgAlert{
			Severity:   "WARNING",
			Metric:     "Connections HWM",
			Threshold:  fmt.Sprintf("> 80%% (%d)", int(float64(maxConn)*0.8)),
			CurrentVal: fmt.Sprintf("%d (%.0f%%)", connCount, float64(connCount)/float64(maxConn)*100),
			Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
			Status:     "LOGGED",
		})
	}

	// Check replication lag on standby
	lag, status, err := c.GetReplicationLag(instanceName)
	if err == nil && status == "standby" && lag > 10 {
		alerts = append(alerts, models.PgAlert{
			Severity:   "CRITICAL",
			Metric:     "Replication Lag",
			Threshold:  "> 10 MB",
			CurrentVal: fmt.Sprintf("%.1f MB", lag),
			Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
			Status:     "ACTIVE",
		})
	}

	// Check for tables with high bloat
	bloatQuery := `
		/* SQL_OPTIMA */ SELECT   
			schemaname || '.' || tablename as table_name,
			n_dead_tup as dead_tuples,
			CASE WHEN n_live_tup > 0 THEN (n_dead_tup::float / n_live_tup) * 100 ELSE 0 END as bloat_pct
		FROM pg_stat_user_tables
		WHERE n_dead_tup > 1000
		ORDER BY n_dead_tup DESC
		LIMIT 3
	`

	bloatRows, err := db.Query(bloatQuery)
	if err == nil {
		defer bloatRows.Close()
		for bloatRows.Next() {
			var tableName string
			var deadTuples int
			var bloatPct float64
			_ = bloatRows.Scan(&tableName, &deadTuples, &bloatPct)
			if bloatPct > 20 {
				alerts = append(alerts, models.PgAlert{
					Severity:   "WARNING",
					Metric:     fmt.Sprintf("Table Bloat (%s)", tableName),
					Threshold:  "> 20%",
					CurrentVal: fmt.Sprintf("%.1f%% (%d dead tuples)", bloatPct, deadTuples),
					Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
					Status:     "LOGGED",
				})
			}
		}
	}

	return alerts, nil
}

// GetGlobalMetric returns global instance metrics for the global dashboard overview.
// Includes connection status and basic resource usage estimates.
func (c *PgRepository) GetGlobalMetric(name string, base models.GlobalInstanceMetric) models.GlobalInstanceMetric {
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

	// PostgreSQL doesn't provide direct OS CPU/memory metrics without extensions
	// These are set to 0 to indicate unavailable data
	base.CPUUsage = 0
	base.MemoryPct = 0

	return base
}
