// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage methods for enhanced PostgreSQL memory intelligence.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/models"
)

// LogPgHostMemory inserts a host memory snapshot into TimescaleDB.
func (tl *TimescaleLogger) LogPgHostMemory(ctx context.Context, snap *models.PgHostMemorySnapshot) error {
	query := `
		INSERT INTO monitor.host_memory_samples (
			ts, server_id, server_instance_name, total_memory_mb, used_memory_mb, 
			free_memory_mb, cached_memory_mb, buffered_memory_mb, swap_total_mb, 
			swap_used_mb, page_faults, major_page_faults
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := tl.pool.Exec(ctx, query,
		snap.Timestamp, snap.ServerID, snap.InstanceName, snap.TotalMemoryMB, snap.UsedMemoryMB,
		snap.FreeMemoryMB, snap.CachedMemoryMB, snap.BufferedMemoryMB, snap.SwapTotalMB,
		snap.SwapUsedMB, snap.PageFaults, snap.MajorPageFaults,
	)
	return err
}

// LogPgMemoryStats inserts a PostgreSQL internal memory snapshot into TimescaleDB.
func (tl *TimescaleLogger) LogPgMemoryStats(ctx context.Context, snap *models.PgMemoryStatsSnapshot) error {
	query := `
		INSERT INTO monitor.pg_memory_samples (
			ts, server_instance_name, postgres_rss_mb, postgres_vsz_mb, 
			active_connections, idle_connections, total_connections, 
			blks_hit, blks_read, temp_files, temp_bytes, 
			buffers_checkpoint, buffers_clean, buffers_backend
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err := tl.pool.Exec(ctx, query,
		snap.Timestamp, snap.InstanceName, snap.PostgresRSSMB, snap.PostgresVSZMB,
		snap.ActiveConnections, snap.IdleConnections, snap.TotalConnections,
		snap.BlksHit, snap.BlksRead, snap.TempFiles, snap.TempBytes,
		snap.BuffersCheckpoint, snap.BuffersClean, snap.BuffersBackend,
	)
	return err
}

// LogPgMemoryComponents inserts a PostgreSQL memory configuration snapshot into TimescaleDB.
func (tl *TimescaleLogger) LogPgMemoryComponents(ctx context.Context, snap *models.PgMemoryComponentsSnapshot) error {
	query := `
		INSERT INTO monitor.pg_memory_components (
			ts, server_instance_name, shared_buffers_mb, work_mem_mb, 
			maintenance_work_mem_mb, wal_buffers_mb, temp_buffers_mb, max_connections
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := tl.pool.Exec(ctx, query,
		snap.Timestamp, snap.InstanceName, snap.SharedBuffersMB, snap.WorkMemMB,
		snap.MaintenanceWorkMemMB, snap.WalBuffersMB, snap.TempBuffersMB, snap.MaxConnections,
	)
	return err
}

// ComputeAndLogPgMemoryDerived calculates and persists memory intelligence metrics.
func (tl *TimescaleLogger) ComputeAndLogPgMemoryDerived(ctx context.Context, instanceName string) error {
	// 1. Fetch latest raw samples and compute derived metrics in one go.
	// We use LATERAL joins to find the most recent matching host and previous PG samples.
	query := `
		INSERT INTO monitor.pg_memory_derived (
			ts, server_instance_name, pg_memory_percent, cache_hit_ratio, 
			temp_spill_rate_mb_s, swap_used_percent, connection_memory_est_mb, 
			memory_pressure_percent, health_score
		)
		SELECT 
			p.ts, 
			p.server_instance_name,
			CASE WHEN COALESCE(h.total_memory_mb, 0) > 0 THEN (COALESCE(p.postgres_rss_mb, 0)::float / h.total_memory_mb) * 100 ELSE 0 END,
			CASE 
				WHEN (COALESCE(p.blks_hit, 0) + COALESCE(p.blks_read, 0)) = 0 THEN 100
				ELSE (p.blks_hit::float / (p.blks_hit + p.blks_read)) * 100 
			END,
			-- Delta temp_bytes over delta time
			CASE 
				WHEN p_prev.ts IS NULL OR (p.ts - p_prev.ts) = INTERVAL '0' THEN 0
				ELSE (COALESCE(p.temp_bytes, 0) - COALESCE(p_prev.temp_bytes, 0))::float / 1024 / 1024 / NULLIF(EXTRACT(EPOCH FROM (p.ts - p_prev.ts)), 0)
			END,
			CASE WHEN COALESCE(h.swap_total_mb, 0) > 0 THEN (COALESCE(h.swap_used_mb, 0)::float / h.swap_total_mb) * 100 ELSE 0 END,
			(COALESCE(p.active_connections, 0) * 10.0), -- estimate 10MB per active connection
			CASE WHEN COALESCE(h.total_memory_mb, 0) > 0 THEN ((COALESCE(h.used_memory_mb, 0) + COALESCE(h.cached_memory_mb, 0))::float / h.total_memory_mb) * 100 ELSE 0 END,
			100 -- Placeholder for health score logic
		FROM (
			SELECT * FROM monitor.pg_memory_samples 
			WHERE UPPER(server_instance_name) = UPPER($1) 
			ORDER BY ts DESC LIMIT 1
		) p
		LEFT JOIN LATERAL (
			SELECT ts, temp_bytes FROM monitor.pg_memory_samples 
			WHERE UPPER(server_instance_name) = UPPER($1) AND ts < p.ts
			ORDER BY ts DESC LIMIT 1
		) p_prev ON TRUE
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.host_memory_samples 
			WHERE UPPER(server_instance_name) = UPPER($1) 
			  AND ts <= p.ts + INTERVAL '30 seconds'
			  AND ts >= p.ts - INTERVAL '30 seconds'
			ORDER BY ABS(EXTRACT(EPOCH FROM (ts - p.ts))) ASC
			LIMIT 1
		) h ON TRUE
		ON CONFLICT (ts, (UPPER(server_instance_name))) DO NOTHING`

	_, err := tl.pool.Exec(ctx, query, instanceName)
	if err != nil {
		log.Printf("[TSLogger] ComputeAndLogPgMemoryDerived error for %s: %v", instanceName, err)
	}
	return err
}

// GetPgMemoryTimeSeries returns time-series data for the memory dashboard.
func (tl *TimescaleLogger) GetPgMemoryTimeSeries(ctx context.Context, instanceName string, from, to time.Time) (map[string]interface{}, error) {
	// Use pg_memory_samples as the primary time-series driver to ensure we show
	// at least raw metrics even if the derived calculation is lagging or failing.
	// h_agent (OS collector agent) join removed: those tables are optional and only
	// present when the os_collector binary is deployed. Depending on them here caused
	// the entire query to fail with "relation does not exist" when script 05 had not
	// been applied. host_memory_samples (populated by CollectPgHostMemory) provides
	// the same OS-level data for the standard deployment path.
	query := `
		SELECT
			p.ts,
			COALESCE(h.used_memory_mb, 0) as used_mb,
			COALESCE(h.free_memory_mb, 0) as free_mb,
			COALESCE(h.cached_memory_mb, 0) as cached_mb,
			COALESCE(h.swap_used_mb, 0) as swap_mb,
			COALESCE(p.postgres_rss_mb, 0),
			COALESCE(d.pg_memory_percent, 0),
			COALESCE(d.cache_hit_ratio,
				CASE WHEN (p.blks_hit + p.blks_read) > 0
					 THEN (p.blks_hit::float / (p.blks_hit + p.blks_read)) * 100
					 ELSE 100 END
			) as cache_hit_ratio,
			COALESCE(d.temp_spill_rate_mb_s, 0),
			COALESCE(d.memory_pressure_percent,
				CASE WHEN h.total_memory_mb > 0
					 THEN ((h.used_memory_mb + h.cached_memory_mb)::float / h.total_memory_mb) * 100
					 ELSE 0 END
			) as memory_pressure_percent,
			COALESCE(d.health_score, 100),
			COALESCE(h.total_memory_mb, 0) as total_mb,
			COALESCE(p.active_connections, 0),
			COALESCE(p.total_connections, 0)
		FROM monitor.pg_memory_samples p
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.pg_memory_derived d2
			WHERE UPPER(d2.server_instance_name) = UPPER(p.server_instance_name)
			  AND d2.ts <= p.ts + INTERVAL '10 seconds'
			  AND d2.ts >= p.ts - INTERVAL '10 seconds'
			ORDER BY ABS(EXTRACT(EPOCH FROM (d2.ts - p.ts))) ASC
			LIMIT 1
		) d ON TRUE
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.host_memory_samples h2
			WHERE UPPER(h2.server_instance_name) = UPPER(p.server_instance_name)
			  AND h2.ts <= p.ts + INTERVAL '30 seconds'
			  AND h2.ts >= p.ts - INTERVAL '30 seconds'
			ORDER BY ABS(EXTRACT(EPOCH FROM (h2.ts - p.ts))) ASC
			LIMIT 1
		) h ON TRUE
		WHERE UPPER(p.server_instance_name) = UPPER($1) AND p.ts >= $2 AND p.ts <= $3
		ORDER BY p.ts ASC`
	rows, err := tl.pool.Query(ctx, query, instanceName, from, to)
	if err != nil {
		log.Printf("[TSLogger] GetPgMemoryTimeSeries query error for %s: %v", instanceName, err)
		return nil, err
	}
	defer rows.Close()

	points := make([]map[string]interface{}, 0)
	for rows.Next() {
		var ts time.Time
		var used, free, cached, swap, rss, total, active, totalConn int64
		var pgPct, cacheHit, tempRate, pressure float64
		var score int
		if err := rows.Scan(&ts, &used, &free, &cached, &swap, &rss, &pgPct, &cacheHit, &tempRate, &pressure, &score, &total, &active, &totalConn); err != nil {
			log.Printf("[TSLogger] GetPgMemoryTimeSeries scan error for %s: %v", instanceName, err)
			continue
		}
		points = append(points, map[string]interface{}{
			"ts":                      ts,
			"used_mb":                 used,
			"free_mb":                 free,
			"cached_mb":               cached,
			"swap_used_mb":            swap,
			"postgres_rss_mb":         rss,
			"pg_memory_percent":       pgPct,
			"cache_hit_ratio":         cacheHit,
			"temp_spill_rate_mb_s":    tempRate,
			"memory_pressure_percent": pressure,
			"health_score":            score,
			"total_mem_mb":            total,
			"active_connections":      active,
			"total_connections":       totalConn,
		})
	}

	log.Printf("[TSLogger] GetPgMemoryTimeSeries: %s returned %d points for range [%v - %v]", instanceName, len(points), from, to)

	// 2. Memory Components (latest)
	var comp models.PgMemoryComponentsSnapshot
	err = tl.pool.QueryRow(ctx, `
		SELECT shared_buffers_mb, work_mem_mb, maintenance_work_mem_mb, wal_buffers_mb, temp_buffers_mb, max_connections
		FROM monitor.pg_memory_components
		WHERE UPPER(server_instance_name) = UPPER($1)
		ORDER BY ts DESC LIMIT 1`, instanceName).Scan(
		&comp.SharedBuffersMB, &comp.WorkMemMB, &comp.MaintenanceWorkMemMB, &comp.WalBuffersMB, &comp.TempBuffersMB, &comp.MaxConnections,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("[TSLogger] GetPgMemoryTimeSeries components query error for %s: %v", instanceName, err)
	}

	return map[string]interface{}{
		"time_series": points,
		"components":  comp,
	}, nil
}
