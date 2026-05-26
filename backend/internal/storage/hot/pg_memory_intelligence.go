// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage methods for enhanced PostgreSQL memory intelligence.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"log/slog"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rsharma155/sql_optima/internal/models"
)

func pgMemoryEmptyResponse() map[string]interface{} {
	return map[string]interface{}{
		"time_series":             []interface{}{},
		"components":              models.PgMemoryComponentsSnapshot{},
		"os_collector_configured": false,
	}
}

func isPgMemorySchemaMissing(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01" // undefined_table
	}
	return err != nil && strings.Contains(err.Error(), "does not exist")
}

// LogPgHostMemory inserts a host memory snapshot into TimescaleDB.
func (tl *TimescaleLogger) LogPgHostMemory(ctx context.Context, snap *models.PgHostMemorySnapshot) error {
	query := `
		INSERT INTO monitor.host_memory_samples (
			capture_timestamp, server_id, total_memory_mb, used_memory_mb,
			free_memory_mb, cached_memory_mb, buffered_memory_mb, swap_total_mb,
			swap_used_mb, page_faults, major_page_faults
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := tl.pool.Exec(ctx, query,
		snap.Timestamp, snap.ServerID, snap.TotalMemoryMB, snap.UsedMemoryMB,
		snap.FreeMemoryMB, snap.CachedMemoryMB, snap.BufferedMemoryMB, snap.SwapTotalMB,
		snap.SwapUsedMB, snap.PageFaults, snap.MajorPageFaults,
	)
	return err
}

// LogPgMemoryStats inserts a PostgreSQL internal memory snapshot into TimescaleDB.
func (tl *TimescaleLogger) LogPgMemoryStats(ctx context.Context, snap *models.PgMemoryStatsSnapshot) error {
	query := `
		INSERT INTO monitor.pg_memory_samples (
			capture_timestamp, server_id, postgres_rss_mb, postgres_vsz_mb,
			active_connections, idle_connections, total_connections,
			blks_hit, blks_read, temp_files, temp_bytes,
			buffers_checkpoint, buffers_clean, buffers_backend
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err := tl.pool.Exec(ctx, query,
		snap.Timestamp, snap.ServerID, snap.PostgresRSSMB, snap.PostgresVSZMB,
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
			capture_timestamp, server_id, shared_buffers_mb, work_mem_mb,
			maintenance_work_mem_mb, wal_buffers_mb, temp_buffers_mb,
			effective_cache_size_mb, max_connections
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := tl.pool.Exec(ctx, query,
		snap.Timestamp, snap.ServerID, snap.SharedBuffersMB, snap.WorkMemMB,
		snap.MaintenanceWorkMemMB, snap.WalBuffersMB, snap.TempBuffersMB,
		snap.EffectiveCacheSizeMB, snap.MaxConnections,
	)
	return err
}

// ComputeAndLogPgMemoryDerived calculates and persists memory intelligence metrics.
func (tl *TimescaleLogger) ComputeAndLogPgMemoryDerived(ctx context.Context, serverID uuid.UUID) error {
	query := `
		INSERT INTO monitor.pg_memory_derived (
			capture_timestamp, server_id, pg_memory_percent, cache_hit_ratio,
			temp_spill_rate_mb_s, swap_used_percent, connection_memory_est_mb,
			memory_pressure_percent, health_score
		)
		SELECT
			p.capture_timestamp,
			p.server_id,
			CASE WHEN COALESCE(h.total_memory_mb, 0) > 0
				THEN (COALESCE(p.postgres_rss_mb, 0)::float / h.total_memory_mb) * 100
				ELSE 0 END,
			CASE
				WHEN (COALESCE(p.blks_hit, 0) + COALESCE(p.blks_read, 0)) = 0 THEN 100
				ELSE (p.blks_hit::float / (p.blks_hit + p.blks_read)) * 100
			END,
			-- Delta temp_bytes / delta time → MB/s spill rate
			CASE
				WHEN p_prev.capture_timestamp IS NULL
				  OR (p.capture_timestamp - p_prev.capture_timestamp) = INTERVAL '0'
				  OR COALESCE(p.temp_bytes, 0) < COALESCE(p_prev.temp_bytes, 0)
				THEN 0
				ELSE (COALESCE(p.temp_bytes, 0) - COALESCE(p_prev.temp_bytes, 0))::float
				     / 1048576.0
				     / NULLIF(EXTRACT(EPOCH FROM (p.capture_timestamp - p_prev.capture_timestamp)), 0)
			END,
			CASE WHEN COALESCE(h.swap_total_mb, 0) > 0
				THEN (COALESCE(h.swap_used_mb, 0)::float / h.swap_total_mb) * 100
				ELSE 0 END,
			(COALESCE(p.active_connections, 0) * 10.0),
			CASE WHEN COALESCE(h.total_memory_mb, 0) > 0
				THEN ((COALESCE(h.used_memory_mb, 0) + COALESCE(h.cached_memory_mb, 0))::float
				     / h.total_memory_mb) * 100
				ELSE 0 END,
			100
		FROM (
			SELECT * FROM monitor.pg_memory_samples
			WHERE server_id = $1
			ORDER BY capture_timestamp DESC LIMIT 1
		) p
		LEFT JOIN LATERAL (
			SELECT capture_timestamp, temp_bytes
			FROM monitor.pg_memory_samples
			WHERE server_id = $1 AND capture_timestamp < p.capture_timestamp
			ORDER BY capture_timestamp DESC LIMIT 1
		) p_prev ON TRUE
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.host_memory_samples
			WHERE server_id = $1
			  AND capture_timestamp BETWEEN p.capture_timestamp - INTERVAL '30 seconds'
			                            AND p.capture_timestamp + INTERVAL '30 seconds'
			ORDER BY ABS(EXTRACT(EPOCH FROM (capture_timestamp - p.capture_timestamp))) ASC
			LIMIT 1
		) h ON TRUE
		ON CONFLICT (capture_timestamp, server_id) DO NOTHING`

	_, err := tl.pool.Exec(ctx, query, serverID)
	if err != nil {
		slog.Error("[TSLogger] ComputeAndLogPgMemoryDerived error", "target", serverID, "err", err)
	}
	return err
}

// GetPgMemoryTimeSeries returns time-series data for the memory intelligence dashboard.
// Timeline merges pg_memory_samples (always collected from the instance) with host_memory_samples
// (os_collector only) so PG cache/connection/spill metrics appear without the OS agent.
func (tl *TimescaleLogger) GetPgMemoryTimeSeries(ctx context.Context, serverID uuid.UUID, from, to time.Time) (map[string]interface{}, error) {
	query := `
		WITH timeline AS (
			SELECT capture_timestamp AS ts, server_id
			FROM monitor.pg_memory_samples
			WHERE server_id = $1
			  AND capture_timestamp >= $2
			  AND capture_timestamp <= $3
			UNION
			SELECT capture_timestamp, server_id
			FROM monitor.host_memory_samples
			WHERE server_id = $1
			  AND capture_timestamp >= $2
			  AND capture_timestamp <= $3
		)
		SELECT
			t.ts,
			COALESCE(h.used_memory_mb, 0)                           AS used_mb,
			COALESCE(h.free_memory_mb, 0)                           AS free_mb,
			COALESCE(h.cached_memory_mb, 0)                         AS cached_mb,
			COALESCE(h.swap_used_mb, 0)                             AS swap_mb,
			COALESCE(p.postgres_rss_mb,
				(SELECT (pm.postgres_rss_bytes / 1048576)::bigint
				 FROM monitor.pg_os_process_memory pm
				 JOIN monitor.pg_os_host_instance ohi ON pm.host_id = ohi.host_id
				 WHERE ohi.server_id = t.server_id
				   AND pm.capture_timestamp BETWEEN t.ts - INTERVAL '2 minutes'
				                                AND t.ts + INTERVAL '2 minutes'
				 ORDER BY ABS(EXTRACT(EPOCH FROM (pm.capture_timestamp - t.ts))) ASC
				 LIMIT 1), 0)                                          AS rss_mb,
			COALESCE(d.pg_memory_percent, 0)                        AS pg_mem_pct,
			COALESCE(d.cache_hit_ratio,
				CASE WHEN (COALESCE(p.blks_hit,0) + COALESCE(p.blks_read,0)) > 0
					THEN (p.blks_hit::float / (p.blks_hit + p.blks_read)) * 100
					ELSE 100 END
			)                                                        AS cache_hit_ratio,
			COALESCE(d.temp_spill_rate_mb_s, 0)                     AS temp_spill,
			COALESCE(d.memory_pressure_percent,
				CASE WHEN COALESCE(h.total_memory_mb,0) > 0
					THEN ((h.used_memory_mb + h.cached_memory_mb)::float / h.total_memory_mb) * 100
					ELSE 0 END
			)                                                        AS pressure,
			COALESCE(d.health_score, 100)                           AS health_score,
			COALESCE(h.total_memory_mb, 0)                          AS total_mb,
			COALESCE(p.active_connections, 0)                       AS active_conn,
			COALESCE(p.idle_connections, 0)                         AS idle_conn,
			COALESCE(p.total_connections, 0)                        AS total_conn,
			COALESCE(p.blks_hit, 0)                                 AS blks_hit,
			COALESCE(p.blks_read, 0)                                AS blks_read,
			COALESCE(p.temp_files, 0)                               AS temp_files,
			COALESCE(p.buffers_checkpoint, 0)                       AS buf_checkpoint,
			COALESCE(p.buffers_clean, 0)                            AS buf_clean,
			COALESCE(p.buffers_backend, 0)                          AS buf_backend
		FROM timeline t
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.host_memory_samples h
			WHERE h.server_id = t.server_id
			  AND h.capture_timestamp BETWEEN t.ts - INTERVAL '30 seconds'
			                              AND t.ts + INTERVAL '30 seconds'
			ORDER BY ABS(EXTRACT(EPOCH FROM (h.capture_timestamp - t.ts))) ASC
			LIMIT 1
		) h ON TRUE
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.pg_memory_samples p
			WHERE p.server_id = t.server_id
			  AND p.capture_timestamp BETWEEN t.ts - INTERVAL '30 seconds'
			                              AND t.ts + INTERVAL '30 seconds'
			ORDER BY ABS(EXTRACT(EPOCH FROM (p.capture_timestamp - t.ts))) ASC
			LIMIT 1
		) p ON TRUE
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.pg_memory_derived d2
			WHERE d2.server_id = t.server_id
			  AND d2.capture_timestamp BETWEEN t.ts - INTERVAL '10 seconds'
			                               AND t.ts + INTERVAL '10 seconds'
			ORDER BY ABS(EXTRACT(EPOCH FROM (d2.capture_timestamp - t.ts))) ASC
			LIMIT 1
		) d ON TRUE
		WHERE t.server_id = $1
		ORDER BY t.ts ASC`

	rows, err := tl.pool.Query(ctx, query, serverID, from, to)
	if err != nil {
		if isPgMemorySchemaMissing(err) {
			slog.Warn("[TSLogger] GetPgMemoryTimeSeries: monitor memory tables missing — apply schema scripts 01 and 05", "target", serverID, "err", err)
			return pgMemoryEmptyResponse(), nil
		}
		slog.Error("[TSLogger] GetPgMemoryTimeSeries query error", "target", serverID, "err", err)
		return nil, err
	}
	defer rows.Close()

	points := make([]map[string]interface{}, 0)
	osConfigured := false

	for rows.Next() {
		var ts time.Time
		var used, free, cached, swap, rss, total, activeConn, idleConn, totalConn int64
		var blksHit, blksRead, tempFiles, bufCkpt, bufClean, bufBackend int64
		var pgPct, cacheHit, tempRate, pressure float64
		var score int

		if err := rows.Scan(
			&ts, &used, &free, &cached, &swap, &rss,
			&pgPct, &cacheHit, &tempRate, &pressure, &score,
			&total, &activeConn, &idleConn, &totalConn,
			&blksHit, &blksRead, &tempFiles,
			&bufCkpt, &bufClean, &bufBackend,
		); err != nil {
			slog.Error("[TSLogger] GetPgMemoryTimeSeries scan error", "target", serverID, "err", err)
			continue
		}

		if total > 0 || rss > 0 {
			osConfigured = true
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
			"active_connections":      activeConn,
			"idle_connections":        idleConn,
			"total_connections":       totalConn,
			"blks_hit":                blksHit,
			"blks_read":               blksRead,
			"temp_files":              tempFiles,
			"buffers_checkpoint":      bufCkpt,
			"buffers_clean":           bufClean,
			"buffers_backend":         bufBackend,
		})
	}
	if err := rows.Err(); err != nil {
		slog.Error("[TSLogger] GetPgMemoryTimeSeries rows error", "target", serverID, "err", err)
		return nil, err
	}

	// Fetch latest memory components (GUC config).
	var comp models.PgMemoryComponentsSnapshot
	err = tl.pool.QueryRow(ctx, `
		SELECT shared_buffers_mb, work_mem_mb, maintenance_work_mem_mb,
		       wal_buffers_mb, temp_buffers_mb,
		       COALESCE(effective_cache_size_mb, 0), max_connections
		FROM monitor.pg_memory_components
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC LIMIT 1`, serverID).Scan(
		&comp.SharedBuffersMB, &comp.WorkMemMB, &comp.MaintenanceWorkMemMB,
		&comp.WalBuffersMB, &comp.TempBuffersMB,
		&comp.EffectiveCacheSizeMB, &comp.MaxConnections,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Error("[TSLogger] GetPgMemoryTimeSeries components query error", "target", serverID, "err", err)
	}

	if !osConfigured {
		if hasRecent, err := tl.HasRecentOSCollectorSamples(ctx, serverID, 20*time.Minute); err == nil && hasRecent {
			osConfigured = true
		}
	}

	return map[string]interface{}{
		"time_series":             points,
		"components":              comp,
		"os_collector_configured": osConfigured,
	}, nil
}

// HasRecentHostMemorySamples returns true if host RAM was ingested from the OS collector within the lookback window.
func (tl *TimescaleLogger) HasRecentHostMemorySamples(ctx context.Context, serverID uuid.UUID, within time.Duration) (bool, error) {
	return tl.HasRecentOSCollectorSamples(ctx, serverID, within)
}

// HasRecentOSCollectorSamples returns true if os_collector pushed host RAM or OS CPU samples recently.
func (tl *TimescaleLogger) HasRecentOSCollectorSamples(ctx context.Context, serverID uuid.UUID, within time.Duration) (bool, error) {
	if tl == nil || tl.pool == nil || serverID == uuid.Nil {
		return false, nil
	}
	if within <= 0 {
		within = 20 * time.Minute
	}
	since := time.Now().UTC().Add(-within)
	var ok bool
	err := tl.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM monitor.host_memory_samples
			WHERE server_id = $1 AND capture_timestamp >= $2
		) OR EXISTS (
			SELECT 1 FROM monitor.pg_os_cpu_samples c
			JOIN monitor.pg_os_host_instance h ON c.host_id = h.host_id
			WHERE h.server_id = $1 AND c.capture_timestamp >= $2
		)`, serverID, since).Scan(&ok)
	if err != nil {
		if isPgMemorySchemaMissing(err) {
			return false, nil
		}
		return false, err
	}
	return ok, nil
}
