// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL memory intelligence domain models.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import "time"

// PgHostMemorySnapshot holds OS-level memory metrics.
type PgHostMemorySnapshot struct {
	Timestamp        time.Time `json:"ts"`
	ServerID         string    `json:"server_id"`
	InstanceName     string    `json:"instance_name"`
	TotalMemoryMB    int64     `json:"total_memory_mb"`
	UsedMemoryMB     int64     `json:"used_memory_mb"`
	FreeMemoryMB     int64     `json:"free_memory_mb"`
	CachedMemoryMB   int64     `json:"cached_memory_mb"`
	BufferedMemoryMB int64     `json:"buffered_memory_mb"`
	SwapTotalMB      int64     `json:"swap_total_mb"`
	SwapUsedMB       int64     `json:"swap_used_mb"`
	PageFaults       int64     `json:"page_faults"`
	MajorPageFaults  int64     `json:"major_page_faults"`
}

// PgMemoryStatsSnapshot holds PostgreSQL internal memory usage metrics.
type PgMemoryStatsSnapshot struct {
	Timestamp         time.Time `json:"ts"`
	InstanceName      string    `json:"instance_name"`
	PostgresRSSMB     int64     `json:"postgres_rss_mb"`
	PostgresVSZMB     int64     `json:"postgres_vsz_mb"`
	ActiveConnections int       `json:"active_connections"`
	IdleConnections   int       `json:"idle_connections"`
	TotalConnections  int       `json:"total_connections"`
	BlksHit           int64     `json:"blks_hit"`
	BlksRead          int64     `json:"blks_read"`
	TempFiles         int64     `json:"temp_files"`
	TempBytes         int64     `json:"temp_bytes"`
	BuffersCheckpoint int64     `json:"buffers_checkpoint"`
	BuffersClean      int64     `json:"buffers_clean"`
	BuffersBackend    int64     `json:"buffers_backend"`
}

// PgMemoryComponentsSnapshot holds PostgreSQL memory configuration settings.
type PgMemoryComponentsSnapshot struct {
	Timestamp            time.Time `json:"ts"`
	InstanceName         string    `json:"instance_name"`
	SharedBuffersMB      int64     `json:"shared_buffers_mb"`
	WorkMemMB            int64     `json:"work_mem_mb"`
	MaintenanceWorkMemMB int64     `json:"maintenance_work_mem_mb"`
	WalBuffersMB         int64     `json:"wal_buffers_mb"`
	TempBuffersMB        int64     `json:"temp_buffers_mb"`
	MaxConnections       int       `json:"max_connections"`
}

// PgMemoryDerivedMetrics holds calculated memory intelligence metrics.
type PgMemoryDerivedMetrics struct {
	Timestamp             time.Time `json:"ts"`
	InstanceName          string    `json:"instance_name"`
	PgMemoryPercent       float64   `json:"pg_memory_percent"`
	CacheHitRatio         float64   `json:"cache_hit_ratio"`
	TempSpillRateMBS      float64   `json:"temp_spill_rate_mb_s"`
	SwapUsedPercent       float64   `json:"swap_used_percent"`
	ConnectionMemoryEstMB float64   `json:"connection_memory_est_mb"`
	MemoryPressurePercent float64   `json:"memory_pressure_percent"`
	HealthScore           int       `json:"health_score"`
}
