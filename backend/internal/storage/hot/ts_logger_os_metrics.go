package hot

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// SaveOSMetrics persists host-level OS telemetry.
func (tl *TimescaleLogger) SaveOSMetrics(ctx context.Context, hostname, instanceName string, m map[string]interface{}) error {
	// 1. Ensure host exists and get host_id
	var hostID int64
	err := tl.pool.QueryRow(ctx, `
		INSERT INTO monitor.pg_os_host_instance (hostname, total_memory_bytes, cpu_cores)
		VALUES ($1, $2, $3)
		ON CONFLICT (hostname) DO UPDATE SET
			total_memory_bytes = EXCLUDED.total_memory_bytes,
			cpu_cores = EXCLUDED.cpu_cores
		RETURNING host_id`,
		hostname, m["total_bytes"], m["cpu_cores"],
	).Scan(&hostID)
	if err != nil {
		return fmt.Errorf("failed to sync pg_os_host_instance: %w", err)
	}

	ts := m["ts"].(time.Time)

	// 2. Insert OS memory samples
	_, err = tl.pool.Exec(ctx, `
		INSERT INTO monitor.pg_os_memory_samples (
			ts, host_id, total_bytes, available_bytes, used_bytes, free_bytes, 
			cached_bytes, buffers_bytes, shared_bytes, slab_bytes,
			swap_total_bytes, swap_used_bytes, swap_free_bytes,
			dirty_bytes, writeback_bytes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		ts, hostID, m["total_bytes"], m["available_bytes"], m["used_bytes"], m["free_bytes"],
		m["cached_bytes"], m["buffers_bytes"], m["shared_bytes"], m["slab_bytes"],
		m["swap_total_bytes"], m["swap_used_bytes"], m["swap_free_bytes"],
		m["dirty_bytes"], m["writeback_bytes"],
	)
	if err != nil {
		return fmt.Errorf("failed to insert pg_os_memory_samples: %w", err)
	}

	// 3. Insert OS memory pressure (derived)
	totalBytes := m["total_bytes"].(uint64)
	if totalBytes > 0 {
		usedPct := float64(m["used_bytes"].(uint64)) * 100.0 / float64(totalBytes)
		availPct := float64(m["available_bytes"].(uint64)) * 100.0 / float64(totalBytes)

		var swapUsedPct float64
		swapTotal := m["swap_total_bytes"].(uint64)
		if swapTotal > 0 {
			swapUsedPct = float64(m["swap_used_bytes"].(uint64)) * 100.0 / float64(swapTotal)
		}

		pressureScore := (usedPct * 0.4) + (swapUsedPct * 0.3)

		_, err = tl.pool.Exec(ctx, `
			INSERT INTO monitor.pg_os_memory_pressure (
				ts, host_id, memory_used_pct, memory_available_pct, swap_used_pct, pressure_score
			) VALUES ($1, $2, $3, $4, $5, $6)`,
			ts, hostID, usedPct, availPct, swapUsedPct, pressureScore,
		)
		if err != nil {
			return fmt.Errorf("failed to insert pg_os_memory_pressure: %w", err)
		}
	}

	// 4. Insert Postgres process memory
	if m["postgres_rss_bytes"] != nil {
		_, err = tl.pool.Exec(ctx, `
			INSERT INTO monitor.pg_os_process_memory (
				ts, host_id, postgres_rss_bytes, postgres_vsz_bytes, backend_count
			) VALUES ($1, $2, $3, $4, $5)`,
			ts, hostID, m["postgres_rss_bytes"], m["postgres_vsz_bytes"], m["backend_count"],
		)
		if err != nil {
			return fmt.Errorf("failed to insert pg_os_process_memory: %w", err)
		}
	}

	// 5. Insert CPU samples
	_, err = tl.pool.Exec(ctx, `
		INSERT INTO monitor.pg_os_cpu_samples (
			ts, host_id, cpu_user_pct, cpu_system_pct, cpu_idle_pct, cpu_iowait_pct,
			load_1m, load_5m, load_15m
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ts, hostID, m["cpu_user_pct"], m["cpu_system_pct"], m["cpu_idle_pct"], m["cpu_iowait_pct"],
		m["load_1m"], m["load_5m"], m["load_15m"],
	)
	if err != nil {
		return fmt.Errorf("failed to insert pg_os_cpu_samples: %w", err)
	}

	return nil
}

// GetLatestOSCPUSaturation returns the latest CPU and load metrics from the OS collector.
func (tl *TimescaleLogger) GetLatestOSCPUSaturation(ctx context.Context, hostname string) (map[string]interface{}, error) {
	var m struct {
		User     float64
		System   float64
		Idle     float64
		IOWait   float64
		Load1m   float64
		Cores    int
		RSS      sql.NullInt64
		Backends sql.NullInt64
	}

	query := `
		SELECT 
			c.cpu_user_pct, c.cpu_system_pct, c.cpu_idle_pct, c.cpu_iowait_pct, c.load_1m,
			h.cpu_cores,
			p.postgres_rss_bytes, p.backend_count
		FROM monitor.pg_os_cpu_samples c
		JOIN monitor.pg_os_host_instance h ON c.host_id = h.host_id
		LEFT JOIN LATERAL (
			SELECT * FROM monitor.pg_os_process_memory p2 
			WHERE p2.host_id = c.host_id AND p2.ts <= c.ts 
			ORDER BY p2.ts DESC LIMIT 1
		) p ON TRUE
		WHERE h.hostname = $1
		ORDER BY c.ts DESC LIMIT 1`

	err := tl.pool.QueryRow(ctx, query, hostname).Scan(
		&m.User, &m.System, &m.Idle, &m.IOWait, &m.Load1m, &m.Cores, &m.RSS, &m.Backends,
	)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"cpu_user_pct":   m.User,
		"cpu_system_pct": m.System,
		"cpu_idle_pct":   m.Idle,
		"cpu_iowait_pct": m.IOWait,
		"load_1m":        m.Load1m,
		"cpu_cores":      m.Cores,
		"pg_rss_mb":      float64(m.RSS.Int64) / 1024 / 1024,
		"pg_backends":    int(m.Backends.Int64),
	}, nil
}

// CheckOSCollectorStatus returns true if OS metrics have been received for the host linked to this instance in the last 5 minutes.
func (tl *TimescaleLogger) CheckOSCollectorStatus(ctx context.Context, hostname string) (bool, error) {
	var exists bool
	err := tl.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM monitor.pg_os_memory_samples s
			JOIN monitor.pg_os_host_instance h ON s.host_id = h.host_id
			WHERE h.hostname = $1 AND s.ts > now() - INTERVAL '5 minutes'
		)`, hostname).Scan(&exists)
	return exists, err
}
