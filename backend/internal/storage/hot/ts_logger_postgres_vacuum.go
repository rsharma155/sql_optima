// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL vacuum progress and history logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"github.com/google/uuid"
	"time"

	"github.com/jackc/pgx/v5"
)

type PostgresVacuumProgressRow struct {
	CaptureTimestamp time.Time `json:"capture_timestamp"`
	ServerID         uuid.UUID `json:"server_id"`
	PID              int64     `json:"pid"`
	DatabaseName     string    `json:"database_name,omitempty"`
	UserName         string    `json:"user_name,omitempty"`
	RelationName     string    `json:"relation_name,omitempty"`
	Phase            string    `json:"phase,omitempty"`
	HeapBlksTotal    int64     `json:"heap_blks_total"`
	HeapBlksScanned  int64     `json:"heap_blks_scanned"`
	HeapBlksVacuumed int64     `json:"heap_blks_vacuumed"`
	IndexVacuumCount int64     `json:"index_vacuum_count"`
	MaxDeadTuples    int64     `json:"max_dead_tuples"`
	NumDeadTuples    int64     `json:"num_dead_tuples"`
}

func (tl *TimescaleLogger) LogPostgresVacuumProgress(ctx context.Context, serverID uuid.UUID, rows []PostgresVacuumProgressRow) error {
	if len(rows) == 0 {
		return nil
	}

	sig := pgFnv64(serverID, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig, r.PID, r.DatabaseName, r.RelationName, r.Phase, r.HeapBlksScanned)
	}

	tl.mu.Lock()
	if prev, ok := tl.prevPgVacuumProgressHash[serverID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgVacuumProgressHash[serverID] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_vacuum_progress (
			capture_timestamp, server_id,
			pid, database_name, user_name, relation_name, phase,
			heap_blks_total, heap_blks_scanned, heap_blks_vacuumed,
			index_vacuum_count, max_dead_tuples, num_dead_tuples
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q,
			now, serverID,
			r.PID, r.DatabaseName, r.UserName, r.RelationName, r.Phase,
			r.HeapBlksTotal, r.HeapBlksScanned, r.HeapBlksVacuumed,
			r.IndexVacuumCount, r.MaxDeadTuples, r.NumDeadTuples,
		)
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

func (tl *TimescaleLogger) GetPostgresVacuumProgress(ctx context.Context, serverID uuid.UUID, limit int) ([]PostgresVacuumProgressRow, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `
		SELECT capture_timestamp, server_id,
		       pid, COALESCE(database_name,''), COALESCE(user_name,''), COALESCE(relation_name,''), COALESCE(phase,''),
		       heap_blks_total, heap_blks_scanned, heap_blks_vacuumed,
		       index_vacuum_count, max_dead_tuples, num_dead_tuples
		FROM postgres_vacuum_progress
		WHERE server_id = $1
		ORDER BY capture_timestamp DESC, heap_blks_scanned DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresVacuumProgressRow
	for rows.Next() {
		var r PostgresVacuumProgressRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerID,
			&r.PID, &r.DatabaseName, &r.UserName, &r.RelationName, &r.Phase,
			&r.HeapBlksTotal, &r.HeapBlksScanned, &r.HeapBlksVacuumed,
			&r.IndexVacuumCount, &r.MaxDeadTuples, &r.NumDeadTuples,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
