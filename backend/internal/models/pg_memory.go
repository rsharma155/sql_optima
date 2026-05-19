// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL memory-specific models for intelligence and trend analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package models

import (
	"github.com/google/uuid"
	"time"
)

type PgHostMemorySnapshot struct {
	ServerID         uuid.UUID `json:"server_id"`
	Timestamp        time.Time `json:"capture_timestamp"`
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

type PgMemoryStatsSnapshot struct {
	ServerID          uuid.UUID `json:"server_id"`
	Timestamp         time.Time `json:"capture_timestamp"`
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

type PgMemoryComponentsSnapshot struct {
	ServerID             uuid.UUID `json:"server_id"`
	Timestamp            time.Time `json:"capture_timestamp"`
	SharedBuffersMB      int64     `json:"shared_buffers_mb"`
	WorkMemMB            int64     `json:"work_mem_mb"`
	MaintenanceWorkMemMB int64     `json:"maintenance_work_mem_mb"`
	WalBuffersMB         int64     `json:"wal_buffers_mb"`
	TempBuffersMB        int64     `json:"temp_buffers_mb"`
	EffectiveCacheSizeMB int64     `json:"effective_cache_size_mb"`
	MaxConnections       int       `json:"max_connections"`
}
