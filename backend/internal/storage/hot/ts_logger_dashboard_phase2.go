// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Phase 2 dashboard metrics logger for advanced visualization.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/pkg/dashboard"
)

type RiskHealthRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	BlockingSessions   int       `json:"blocking_sessions"`
	MemoryGrantsPending int      `json:"memory_grants_pending"`
	FailedLogins5m     int       `json:"failed_logins_5m"`
	TempdbUsedPercent  float64   `json:"tempdb_used_percent"`
	MaxLogDbName       string    `json:"max_log_db_name"`
	MaxLogUsedPercent  float64   `json:"max_log_used_percent"`
	PLE                float64   `json:"ple"`
	CompilationsPerSec float64   `json:"compilations_per_sec"`
	BatchReqPerSec     float64   `json:"batch_requests_per_sec"`
	BufferCacheHitPct  float64   `json:"buffer_cache_hit_ratio"`
}

func (tl *TimescaleLogger) LogSQLServerRiskHealth(ctx context.Context, instanceName string, row RiskHealthRow) error {
	q := `
		INSERT INTO sqlserver_risk_health (
			capture_timestamp, server_instance_name,
			blocking_sessions, memory_grants_pending, failed_logins_5m,
			tempdb_used_percent,
			max_log_db_name, max_log_used_percent,
			ple,
			compilations_per_sec, batch_requests_per_sec,
			buffer_cache_hit_ratio
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`
	_, err := tl.pool.Exec(ctx, q,
		row.CaptureTimestamp, instanceName,
		row.BlockingSessions, row.MemoryGrantsPending,
		row.FailedLogins5m,
		row.TempdbUsedPercent,
		row.MaxLogDbName, row.MaxLogUsedPercent,
		row.PLE,
		row.CompilationsPerSec, row.BatchReqPerSec,
		row.BufferCacheHitPct,
	)
	return err
}

func (tl *TimescaleLogger) GetLatestSQLServerRiskHealth(ctx context.Context, instanceName string) (*RiskHealthRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT capture_timestamp, server_instance_name,
		       COALESCE(blocking_sessions,0),
		       COALESCE(memory_grants_pending,0),
		       COALESCE(failed_logins_5m,0),
		       COALESCE(tempdb_used_percent,0),
		       COALESCE(max_log_db_name,''),
		       COALESCE(max_log_used_percent,0),
		       COALESCE(ple,0),
		       COALESCE(compilations_per_sec,0),
		       COALESCE(batch_requests_per_sec,0),
		       COALESCE(buffer_cache_hit_ratio,0)
		FROM sqlserver_risk_health
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var r RiskHealthRow
	err := tl.pool.QueryRow(ctx, q, instanceName).Scan(
		&r.CaptureTimestamp, &r.ServerInstanceName,
		&r.BlockingSessions, &r.MemoryGrantsPending,
		&r.FailedLogins5m,
		&r.TempdbUsedPercent,
		&r.MaxLogDbName, &r.MaxLogUsedPercent,
		&r.PLE,
		&r.CompilationsPerSec, &r.BatchReqPerSec,
		&r.BufferCacheHitPct,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

type WaitDeltaRow struct {
	CaptureTimestamp   time.Time `json:"capture_timestamp"`
	ServerInstanceName string    `json:"server_instance_name"`
	WaitType           string    `json:"wait_type"`
	WaitCategory       string    `json:"wait_category"`
	WaitTimeMsDelta    float64   `json:"wait_time_ms_delta"`
}

func (tl *TimescaleLogger) LogSQLServerWaitDeltas(ctx context.Context, rows []WaitDeltaRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(`
			INSERT INTO sqlserver_waits_delta (
				capture_timestamp, server_instance_name, wait_type, wait_category, wait_time_ms_delta
			) VALUES ($1,$2,$3,$4,$5)
		`, r.CaptureTimestamp, r.ServerInstanceName, r.WaitType, r.WaitCategory, r.WaitTimeMsDelta)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// ComputeAndLogWaitDeltas computes deltas from cumulative waits using internal previous state,
// categorizes them, and persists them into TimescaleDB.
func (tl *TimescaleLogger) ComputeAndLogWaitDeltas(ctx context.Context, instanceName string, currWaitTotals map[string]float64) error {
	if len(currWaitTotals) == 0 {
		return nil
	}

	tl.mu.Lock()
	prev := tl.prevWaitHistory[instanceName]
	if prev == nil {
		prev = make(map[string]float64)
	}
	deltas, nextPrev := dashboard.ComputeWaitDeltas(prev, currWaitTotals)
	tl.prevWaitHistory[instanceName] = nextPrev
	tl.mu.Unlock()

	now := time.Now().UTC()
	// IMPORTANT: Reduce write volume.
	// Instead of inserting one row per wait type (which can be hundreds+ per scrape),
	// persist only category totals. This keeps the dashboard donut accurate while
	// cutting inserts to ~7 rows per interval.
	byCat := map[string]float64{}
	for _, d := range deltas {
		if d.DeltaMs <= 0 {
			continue
		}
		cat := string(d.Category)
		byCat[cat] += d.DeltaMs
	}

	rows := make([]WaitDeltaRow, 0, len(byCat))
	for cat, ms := range byCat {
		if ms <= 0 {
			continue
		}
		rows = append(rows, WaitDeltaRow{
			CaptureTimestamp:   now,
			ServerInstanceName: instanceName,
			WaitType:           "__CATEGORY_TOTAL__",
			WaitCategory:       cat,
			WaitTimeMsDelta:    ms,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	sig := waitDeltaSnapshotFingerprint(instanceName, rows)
	if tl.enterpriseSnapshotUnchanged(instanceName, enterpriseKindWaitsDelta, sig) {
		return nil
	}
	if err := tl.LogSQLServerWaitDeltas(ctx, rows); err != nil {
		return err
	}
	tl.rememberEnterpriseSnapshot(instanceName, enterpriseKindWaitsDelta, sig)
	return nil
}

type WaitCategoryAgg struct {
	WaitCategory string  `json:"wait_category"`
	WaitTimeMs   float64 `json:"wait_time_ms"`
}

// GetWaitCategoryAgg returns summed wait_time_ms_delta per category over the last N minutes.
func (tl *TimescaleLogger) GetWaitCategoryAgg(ctx context.Context, instanceName string, minutes int) ([]WaitCategoryAgg, error) {
	if minutes <= 0 {
		minutes = 15
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT wait_category, SUM(wait_time_ms_delta) AS wait_time_ms
		FROM sqlserver_waits_delta
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - ($2::int * INTERVAL '1 minute')
		GROUP BY wait_category
		ORDER BY wait_time_ms DESC
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, minutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WaitCategoryAgg
	for rows.Next() {
		var r WaitCategoryAgg
		if err := rows.Scan(&r.WaitCategory, &r.WaitTimeMs); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetBufferCacheHitTrend returns 1-minute bucketed avg buffer cache hit ratio (%) for the last N minutes.
// Source: sqlserver_risk_health snapshots (collector-computed).
func (tl *TimescaleLogger) GetBufferCacheHitTrend(ctx context.Context, instanceName string, minutes int) ([]map[string]interface{}, error) {
	if minutes <= 0 {
		minutes = 60
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	q := `
		SELECT time_bucket('1 minute', capture_timestamp) AS bucket,
		       AVG(buffer_cache_hit_ratio) AS buffer_cache_hit_ratio
		FROM sqlserver_risk_health
		WHERE server_instance_name = $1
		  AND capture_timestamp >= NOW() - ($2::int * INTERVAL '1 minute')
		GROUP BY bucket
		ORDER BY bucket ASC
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, minutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, minutes)
	for rows.Next() {
		var ts time.Time
		var v float64
		if err := rows.Scan(&ts, &v); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"timestamp":             ts,
			"buffer_cache_hit_ratio": v,
		})
	}
	return out, rows.Err()
}

