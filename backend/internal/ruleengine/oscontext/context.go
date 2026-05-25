// Package oscontext loads host metrics from the OS collector for rule-engine evaluation.
package oscontext

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultMaxAge is how recent OS samples must be to enrich best-practice rules.
const DefaultMaxAge = 20 * time.Minute

// OS-aware PostgreSQL rule IDs (RAM / host context).
var PostgresOSAwareRuleIDs = map[string]struct{}{
	"PG_SHARED_BUFFERS_001":       {},
	"PG_EFFECTIVE_CACHE_SIZE_001": {},
	"PG_WORK_MEM_001":             {},
	"PG_MAX_CONNECTIONS_001":      {},
	"PG_MAINT_WORK_MEM_001":       {},
}

// RuleEval holds host metrics merged into expr evaluation env.
type RuleEval struct {
	Available        bool
	TotalRAMBytes    float64
	TotalRAMGB       float64
	TotalRAMPages8k  float64
	AvailableBytes   float64
	MemoryUsedPct    float64
	SwapUsedPct      float64
	CPUCores         float64
	PostgresRSSBytes float64
}

// Load reads the latest OS collector snapshot for a server from TimescaleDB.
func Load(ctx context.Context, pool *pgxpool.Pool, serverID uuid.UUID, maxAge time.Duration) (RuleEval, error) {
	var out RuleEval
	if pool == nil || serverID == uuid.Nil {
		return out, nil
	}
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	since := time.Now().UTC().Add(-maxAge)

	var totalMem, avail, used, memUsedPct, swapUsedPct sql.NullFloat64
	var cpuCores sql.NullInt32
	var pgRSS sql.NullInt64

	err := pool.QueryRow(ctx, `
		SELECT
			h.total_memory_bytes,
			h.cpu_cores,
			m.available_bytes,
			m.used_bytes,
			p.memory_used_pct,
			p.swap_used_pct,
			pm.postgres_rss_bytes
		FROM monitor.pg_os_host_instance h
		LEFT JOIN LATERAL (
			SELECT available_bytes, used_bytes
			FROM monitor.pg_os_memory_samples
			WHERE host_id = h.host_id AND capture_timestamp >= $2
			ORDER BY capture_timestamp DESC
			LIMIT 1
		) m ON TRUE
		LEFT JOIN LATERAL (
			SELECT memory_used_pct, swap_used_pct
			FROM monitor.pg_os_memory_pressure
			WHERE host_id = h.host_id AND capture_timestamp >= $2
			ORDER BY capture_timestamp DESC
			LIMIT 1
		) p ON TRUE
		LEFT JOIN LATERAL (
			SELECT postgres_rss_bytes
			FROM monitor.pg_os_process_memory
			WHERE host_id = h.host_id AND capture_timestamp >= $2
			ORDER BY capture_timestamp DESC
			LIMIT 1
		) pm ON TRUE
		WHERE h.server_id = $1
		  AND (
			EXISTS (
				SELECT 1 FROM monitor.pg_os_memory_samples s
				WHERE s.host_id = h.host_id AND s.capture_timestamp >= $2
			)
			OR EXISTS (
				SELECT 1 FROM monitor.pg_os_cpu_samples c
				WHERE c.host_id = h.host_id AND c.capture_timestamp >= $2
			)
		  )
		LIMIT 1`, serverID, since).Scan(
		&totalMem, &cpuCores, &avail, &used, &memUsedPct, &swapUsedPct, &pgRSS,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return out, nil
		}
		if isMissingSchema(err) {
			return out, nil
		}
		return out, err
	}

	if !totalMem.Valid || totalMem.Float64 <= 0 {
		return out, nil
	}

	out.Available = true
	out.TotalRAMBytes = totalMem.Float64
	out.TotalRAMGB = totalMem.Float64 / (1024 * 1024 * 1024)
	out.TotalRAMPages8k = totalMem.Float64 / 8192
	if avail.Valid {
		out.AvailableBytes = avail.Float64
	}
	if memUsedPct.Valid {
		out.MemoryUsedPct = memUsedPct.Float64
	} else if used.Valid && out.TotalRAMBytes > 0 {
		out.MemoryUsedPct = used.Float64 * 100 / out.TotalRAMBytes
	}
	if swapUsedPct.Valid {
		out.SwapUsedPct = swapUsedPct.Float64
	}
	if cpuCores.Valid && cpuCores.Int32 > 0 {
		out.CPUCores = float64(cpuCores.Int32)
	}
	if pgRSS.Valid && pgRSS.Int64 > 0 {
		out.PostgresRSSBytes = float64(pgRSS.Int64)
	}
	return out, nil
}

// MergeInto adds OS fields to the rule evaluation environment (always sets os_available).
func (c RuleEval) MergeInto(env map[string]interface{}) {
	if env == nil {
		return
	}
	if c.Available {
		env["os_available"] = float64(1)
		env["TotalRAM_bytes"] = c.TotalRAMBytes
		env["TotalRAM_GB"] = c.TotalRAMGB
		env["TotalRAM_pages_8k"] = c.TotalRAMPages8k
		env["AvailableRAM_bytes"] = c.AvailableBytes
		env["memory_used_pct"] = c.MemoryUsedPct
		env["swap_used_pct"] = c.SwapUsedPct
		env["cpu_cores"] = c.CPUCores
		env["postgres_rss_bytes"] = c.PostgresRSSBytes
	} else {
		env["os_available"] = float64(0)
		env["TotalRAM_bytes"] = float64(0)
		env["TotalRAM_GB"] = float64(0)
		env["TotalRAM_pages_8k"] = float64(0)
		env["AvailableRAM_bytes"] = float64(0)
		env["memory_used_pct"] = float64(0)
		env["swap_used_pct"] = float64(0)
	}
}

// IsOSAwareRule reports whether a rule should be tagged as OS-enriched when OS data is present.
func IsOSAwareRule(ruleID string) bool {
	_, ok := PostgresOSAwareRuleIDs[ruleID]
	return ok
}

func isMissingSchema(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "42P01")
}
