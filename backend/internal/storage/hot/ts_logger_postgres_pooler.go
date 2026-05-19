// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PgBouncer and connection pooler health metrics logger.
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
)

type PostgresPoolerStatRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	PoolerType       string    `json:"pooler_type"`
	ClActive         int       `json:"cl_active"`
	ClWaiting        int       `json:"cl_waiting"`
	SvActive         int       `json:"sv_active"`
	SvIdle           int       `json:"sv_idle"`
	SvUsed           int       `json:"sv_used"`
	MaxwaitSeconds   float64   `json:"maxwait_seconds"`
	TotalPools       int       `json:"total_pools"`
}

func (tl *TimescaleLogger) LogPostgresPoolerStats(ctx context.Context, serverID uuid.UUID, row PostgresPoolerStatRow) error {
	now := time.Now().UTC()

	sig := pgFnv64(serverID, row.PoolerType, row.ClActive, row.ClWaiting, row.SvActive, row.SvIdle, row.SvUsed, fmt.Sprintf("%.3f", row.MaxwaitSeconds), row.TotalPools)
	key := "pg_pooler|" + serverID.String()
	tl.mu.Lock()
	if prev, ok := tl.prevPgWaitEventsHash[serverID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgWaitEventsHash[serverID] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_pooler_stats (
			capture_timestamp, server_id, pooler_type,
			cl_active, cl_waiting, sv_active, sv_idle, sv_used,
			maxwait_seconds, total_pools
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	_, err := tl.pool.Exec(ctx, q,
		now, serverID, row.PoolerType,
		row.ClActive, row.ClWaiting, row.SvActive, row.SvIdle, row.SvUsed,
		row.MaxwaitSeconds, row.TotalPools,
	)
	_ = key
	return err
}

func (tl *TimescaleLogger) GetLatestPostgresPoolerStats(ctx context.Context, serverID uuid.UUID) (*PostgresPoolerStatRow, error) {
	q := `
		SELECT capture_timestamp, server_id, pooler_type,
		       cl_active, cl_waiting, sv_active, sv_idle, sv_used,
		       maxwait_seconds, total_pools
		FROM postgres_pooler_stats
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var r PostgresPoolerStatRow
	err := tl.pool.QueryRow(ctx, q, serverID).Scan(
		&r.CaptureTimestamp, &r.ServerID, &r.PoolerType,
		&r.ClActive, &r.ClWaiting, &r.SvActive, &r.SvIdle, &r.SvUsed,
		&r.MaxwaitSeconds, &r.TotalPools,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (tl *TimescaleLogger) GetPostgresPoolerHistory(ctx context.Context, serverID uuid.UUID, limit int) ([]PostgresPoolerStatRow, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
		SELECT capture_timestamp, server_id, pooler_type,
		       cl_active, cl_waiting, sv_active, sv_idle, sv_used,
		       maxwait_seconds, total_pools
		FROM postgres_pooler_stats
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresPoolerStatRow
	for rows.Next() {
		var r PostgresPoolerStatRow
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.PoolerType,
			&r.ClActive, &r.ClWaiting, &r.SvActive, &r.SvIdle, &r.SvUsed,
			&r.MaxwaitSeconds, &r.TotalPools); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
