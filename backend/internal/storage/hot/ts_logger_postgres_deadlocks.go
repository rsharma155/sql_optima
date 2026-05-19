// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL deadlock event logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type PostgresDeadlockStatRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	DatabaseName     string    `json:"database_name"`
	DeadlocksTotal   int64     `json:"deadlocks_total"`
	DeadlocksDelta   int64     `json:"deadlocks_delta"`
}

func (tl *TimescaleLogger) LogPostgresDeadlocksDelta(ctx context.Context, serverID uuid.UUID, totals map[string]int64) error {
	if len(totals) == 0 {
		return nil
	}
	now := time.Now().UTC()

	tl.mu.Lock()
	if tl.prevPgDeadlocksTotal == nil {
		tl.prevPgDeadlocksTotal = make(map[uuid.UUID]map[string]int64)
	}
	prevByDB, ok := tl.prevPgDeadlocksTotal[serverID]
	if !ok || prevByDB == nil {
		prevByDB = make(map[string]int64)
		tl.prevPgDeadlocksTotal[serverID] = prevByDB
	}
	// Compute deltas and update prev snapshot
	type row struct {
		db    string
		total int64
		delta int64
	}
	var rows []row
	for db, total := range totals {
		prev := prevByDB[db]
		delta := total - prev
		if prev == 0 {
			// First observation: don't emit a potentially huge historical delta.
			delta = 0
		}
		if delta < 0 {
			// Counter reset (restart/restore). Treat as 0 delta.
			delta = 0
		}
		prevByDB[db] = total
		rows = append(rows, row{db: db, total: total, delta: delta})
	}
	tl.mu.Unlock()

	// If nothing changed (all deltas zero) skip insert.
	allZero := true
	for _, r := range rows {
		if r.delta != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return nil
	}

	q := `
		INSERT INTO postgres_deadlock_stats (
			capture_timestamp, server_id, database_name,
			deadlocks_total, deadlocks_delta
		) VALUES ($1,$2,$3,$4,$5)
	`
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, r.db, r.total, r.delta)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetPostgresDeadlocksHistory(ctx context.Context, serverID uuid.UUID, minutes int, limit int) ([]PostgresDeadlockStatRow, error) {
	if limit <= 0 {
		limit = 180
	}
	if minutes <= 0 {
		minutes = 180
	}
	q := `
		SELECT capture_timestamp, server_id, database_name,
		       deadlocks_total, deadlocks_delta
		FROM postgres_deadlock_stats
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - make_interval(mins => $2)
		ORDER BY capture_timestamp DESC
		LIMIT $3
	`
	rows, err := tl.pool.Query(ctx, q, serverID, minutes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PostgresDeadlockStatRow
	for rows.Next() {
		var r PostgresDeadlockStatRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.DatabaseName, &r.DeadlocksTotal, &r.DeadlocksDelta); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
