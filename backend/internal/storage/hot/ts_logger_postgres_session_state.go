// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL aggregated session state counts logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type PostgresSessionStateRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	ActiveCount      int       `json:"active_count"`
	IdleCount        int       `json:"idle_count"`
	IdleInTxnCount   int       `json:"idle_in_txn_count"`
	WaitingCount     int       `json:"waiting_count"`
	TotalCount       int       `json:"total_count"`
}

func (tl *TimescaleLogger) LogPostgresSessionStateCounts(ctx context.Context, serverID uuid.UUID, active, idle, idleInTxn, waiting, total int) error {
	now := time.Now().UTC()

	sig := pgFnv64(serverID, active, idle, idleInTxn, waiting, total)
	key := "pg_sess_state|" + serverID.String()
	tl.mu.Lock()
	if prev, ok := tl.prevPgSessionHash[serverID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgSessionHash[serverID] = sig
	tl.mu.Unlock()

	q := `INSERT INTO postgres_session_state_counts (capture_timestamp, server_id, active_count, idle_count, idle_in_txn_count, waiting_count, total_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := tl.pool.Exec(ctx, q, now, serverID, active, idle, idleInTxn, waiting, total)
	_ = key
	return err
}

func (tl *TimescaleLogger) GetPostgresSessionStateHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]PostgresSessionStateRow, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
		SELECT capture_timestamp, server_id, active_count, idle_count, idle_in_txn_count, waiting_count, total_count
		FROM postgres_session_state_counts
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresSessionStateRow
	for rows.Next() {
		var r PostgresSessionStateRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.ActiveCount, &r.IdleCount,
			&r.IdleInTxnCount, &r.WaitingCount, &r.TotalCount); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
