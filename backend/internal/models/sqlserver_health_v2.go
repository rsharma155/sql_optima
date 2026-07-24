/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: SQL Server Health Dashboard v2 (Instant Triage) models.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */
package models

import "time"

// HealthV2DashboardResponse is the main container for the V2 dashboard data.
type HealthV2DashboardResponse struct {
	InstanceName      string            `json:"instance_name"`
	DatabaseName      string            `json:"database_name,omitempty"`
	KPIs              HealthV2KPIs      `json:"kpis"`
	WaitTrends        []WaitTrendPoint  `json:"wait_trends"`
	IOLatency         []IOLatencyPoint  `json:"io_latency"`
	Throughput        []ThroughputPoint `json:"throughput"`
	TempDB            TempDBHealth      `json:"tempdb"`
	Problems          ActiveProblems    `json:"problems"`
	LastUpdate        time.Time         `json:"last_update"`
	RefreshIntervalMs int               `json:"refresh_interval_ms"`
	Tooltips          map[string]string `json:"tooltips,omitempty"`
}

type HealthV2KPIs struct {
	SqlCpuPct        float64 `json:"sql_cpu_pct"`
	RunnableTasks    int     `json:"runnable_tasks"`
	MemGrantsPending int     `json:"mem_grants_pending"`
	PageReadsPerSec  float64 `json:"page_reads_per_sec"`
	LogWriteWaitMs   float64 `json:"log_write_wait_ms"`
	BlockedSessions  int     `json:"blocked_sessions"`
	UserConnections  int     `json:"user_connections"`
	MaxConnections   int     `json:"max_connections"`
	BatchRequests    float64 `json:"batch_requests"`
	Compilations          float64 `json:"compilations"`
	LoginsPerSec          float64 `json:"logins_per_sec"`
	TargetServerMemoryMB  float64 `json:"target_server_memory_mb"`
	TotalServerMemoryMB   float64 `json:"total_server_memory_mb"`
	MemoryUtilizationPct  float64 `json:"memory_utilization_pct"`
	DataCacheLifeLifeSec  float64 `json:"data_cache_life_sec"` // Descriptive name for PLE
	StorageMinimumFreePct float64 `json:"storage_minimum_free_pct"`
	VolumesTracked        int     `json:"volumes_tracked"`
	InstanceStatus        string  `json:"instance_status"` // "Healthy", "Warning", "Critical"
	Edition          string  `json:"edition"`
	Uptime           string  `json:"uptime"`
}

type WaitTrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	CPU       float64   `json:"cpu"`
	IO        float64   `json:"io"`
	Memory    float64   `json:"memory"`
	Locking   float64   `json:"locking"`
	Parallel  float64   `json:"parallel"`
	Other     float64   `json:"other"`
}

type IOLatencyPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	DataReadMs  float64   `json:"data_read_ms"`
	DataWriteMs float64   `json:"data_write_ms"`
	LogWriteMs  float64   `json:"log_write_ms"`
	ReadIOPS    float64   `json:"read_iops"`
	WriteIOPS   float64   `json:"write_iops"`
}

type ThroughputPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	BatchRequests float64   `json:"batch_requests"`
	Connections   int       `json:"connections"`
	LoginsPerSec  float64   `json:"logins_per_sec"`
}

type TempDBHealth struct {
	UserObjMB       float64 `json:"user_obj_mb"`
	InternalObjMB   float64 `json:"internal_obj_mb"`
	VersionStoreMB  float64 `json:"version_store_mb"`
	FreeMB          float64 `json:"free_mb"`
	LogUsedMB       float64 `json:"log_used_mb"`
	ContentionFound bool    `json:"contention_found"`
}

type ActiveProblems struct {
	LongRunning []LongRunningQuery `json:"long_running"`
	Blocking    []BlockingNode     `json:"blocking"`
}
