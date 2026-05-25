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

// LogSqlServerPerfCountersV2 writes perf counter rows to sqlserver_perf_counters.
// Unchanged (cntr_value, rate) pairs are skipped in-memory; ON CONFLICT DO NOTHING
// handles same-timestamp re-delivery.
func (tl *TimescaleLogger) LogSqlServerPerfCountersV2(ctx context.Context, serverID uuid.UUID, capturedAt time.Time, rows []PerfCounterWriteRow) error {
	if len(rows) == 0 {
		return nil
	}

	serverPrefix := serverID.String() + ":"
	toInsert := make([]PerfCounterWriteRow, 0, len(rows))
	for _, r := range rows {
		key := serverPrefix + r.CounterName + ":" + r.InstanceName
		newHash := perfCounterWriteHash(r.CntrValue, r.RatePerSec)

		tl.mu.RLock()
		prevHash, ok := tl.prevPerfCounterWriteHash[key]
		tl.mu.RUnlock()

		if ok && prevHash == newHash {
			continue
		}

		tl.mu.Lock()
		tl.prevPerfCounterWriteHash[key] = newHash
		tl.mu.Unlock()

		toInsert = append(toInsert, r)
	}
	if len(toInsert) == 0 {
		return nil
	}

	const q = `
		INSERT INTO sqlserver_perf_counters (
			capture_timestamp, server_id, counter_name, instance_name,
			cntr_value, cntr_type, value_per_sec
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING`

	b := &pgx.Batch{}
	for _, r := range toInsert {
		b.Queue(q,
			capturedAt, serverID, r.CounterName, r.InstanceName,
			r.CntrValue, r.CntrType, r.RatePerSec,
		)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range toInsert {
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
