// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB read/write for the extended sqlserver_perf_counters table.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PerfCounterWriteRow is one row written to sqlserver_perf_counters.
type PerfCounterWriteRow struct {
	CounterName  string
	InstanceName string
	CntrValue    int64
	CntrType     int
	RatePerSec   float64
}

// LogSqlServerPerfCountersV2 writes the full set of perf counter rows (one per
// counter per collection cycle) to sqlserver_perf_counters using the extended
// schema. ON CONFLICT DO NOTHING prevents duplicates on re-delivery.
func (tl *TimescaleLogger) LogSqlServerPerfCountersV2(ctx context.Context, serverID uuid.UUID, capturedAt time.Time, rows []PerfCounterWriteRow) error {
	if len(rows) == 0 {
		return nil
	}
	const q = `
		INSERT INTO sqlserver_perf_counters (
			capture_timestamp, server_id, counter_name, instance_name,
			cntr_value, cntr_type, value_per_sec
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING`

	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q,
			capturedAt, serverID, r.CounterName, r.InstanceName,
			r.CntrValue, r.CntrType, r.RatePerSec,
		)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("LogSqlServerPerfCountersV2 insert failed: %w", err)
		}
	}
	return nil
}

// GetLatestPerfCounter returns the most recent value_per_sec (rate) and raw
// cntr_value for a given counter name and instance name, looking back at most
// 5 minutes. The boolean return is false when no fresh row exists.
func (tl *TimescaleLogger) GetLatestPerfCounter(ctx context.Context, serverID uuid.UUID, counterName, instanceName string) (float64, bool, error) {
	const q = `
		SELECT value_per_sec
		FROM sqlserver_perf_counters
		WHERE server_id    = $1
		  AND counter_name = $2
		  AND instance_name = $3
		  AND capture_timestamp >= NOW() - INTERVAL '5 minutes'
		ORDER BY capture_timestamp DESC
		LIMIT 1`

	var rate float64
	err := tl.pool.QueryRow(ctx, q, serverID, counterName, instanceName).Scan(&rate)
	if err != nil {
		return 0, false, nil //nolint:nilerr // missing row is not an error for callers
	}
	return rate, true, nil
}
