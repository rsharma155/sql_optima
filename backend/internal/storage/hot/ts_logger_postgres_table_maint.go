// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL table maintenance history logger.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type PostgresTableMaintRow struct {
	CaptureTimestamp   time.Time  `json:"capture_timestamp"`
	ServerInstanceName string     `json:"server_instance_name"`
	SchemaName         string     `json:"schema_name"`
	TableName          string     `json:"table_name"`
	TotalBytes         int64      `json:"total_bytes"`
	LiveTuples         int64      `json:"live_tuples"`
	DeadTuples         int64      `json:"dead_tuples"`
	DeadPct            float64    `json:"dead_pct"`
	SeqScans           int64      `json:"seq_scans"`
	IdxScans           int64      `json:"idx_scans"`
	LastVacuum         *time.Time `json:"last_vacuum,omitempty"`
	LastAutovacuum     *time.Time `json:"last_autovacuum,omitempty"`
	LastAnalyze        *time.Time `json:"last_analyze,omitempty"`
	LastAutoanalyze    *time.Time `json:"last_autoanalyze,omitempty"`
}

func (tl *TimescaleLogger) LogPostgresTableMaintenance(ctx context.Context, instanceName string, rows []PostgresTableMaintRow) error {
	if len(rows) == 0 {
		return nil
	}

	// Dedup snapshot (top tables only) to avoid identical repeats.
	sig := pgFnv64(instanceName, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig, r.SchemaName, r.TableName, r.TotalBytes, r.LiveTuples, r.DeadTuples, fmt.Sprintf("%.3f", r.DeadPct))
	}
	tl.mu.Lock()
	if tl.prevEnterpriseBatchHash == nil {
		tl.prevEnterpriseBatchHash = make(map[string]uint64)
	}
	key := "pg_tblmaint|" + instanceName
	if prev, ok := tl.prevEnterpriseBatchHash[key]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevEnterpriseBatchHash[key] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_table_maintenance_stats (
			capture_timestamp, server_instance_name,
			schema_name, table_name,
			total_bytes, live_tuples, dead_tuples, dead_pct,
			seq_scans, idx_scans,
			last_vacuum, last_autovacuum, last_analyze, last_autoanalyze
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q,
			now, instanceName,
			r.SchemaName, r.TableName,
			r.TotalBytes, r.LiveTuples, r.DeadTuples, r.DeadPct,
			r.SeqScans, r.IdxScans,
			r.LastVacuum, r.LastAutovacuum, r.LastAnalyze, r.LastAutoanalyze,
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

func (tl *TimescaleLogger) GetPostgresTableMaintenanceHistory(ctx context.Context, instanceName string, schema, table string, limit int) ([]PostgresTableMaintRow, error) {
	if limit <= 0 {
		limit = 180
	}
	q := `
		SELECT capture_timestamp, server_instance_name,
		       schema_name, table_name,
		       total_bytes, live_tuples, dead_tuples, dead_pct,
		       seq_scans, idx_scans,
		       last_vacuum, last_autovacuum, last_analyze, last_autoanalyze
		FROM postgres_table_maintenance_stats
		WHERE server_instance_name = $1
		  AND schema_name = $2
		  AND table_name = $3
		ORDER BY capture_timestamp DESC
		LIMIT $4
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, schema, table, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresTableMaintRow
	for rows.Next() {
		var r PostgresTableMaintRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName,
			&r.SchemaName, &r.TableName,
			&r.TotalBytes, &r.LiveTuples, &r.DeadTuples, &r.DeadPct,
			&r.SeqScans, &r.IdxScans,
			&r.LastVacuum, &r.LastAutovacuum, &r.LastAnalyze, &r.LastAutoanalyze,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) GetLatestPostgresTableMaintenance(ctx context.Context, instanceName string, limit int) ([]PostgresTableMaintRow, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		WITH latest AS (
			SELECT DISTINCT ON (schema_name, table_name)
			       capture_timestamp, server_instance_name,
			       schema_name, table_name,
			       total_bytes, live_tuples, dead_tuples, dead_pct,
			       seq_scans, idx_scans,
			       last_vacuum, last_autovacuum, last_analyze, last_autoanalyze
			FROM postgres_table_maintenance_stats
			WHERE server_instance_name = $1
			ORDER BY schema_name, table_name, capture_timestamp DESC
		)
		SELECT *
		FROM latest
		ORDER BY (total_bytes * (dead_pct/100.0)) DESC NULLS LAST, dead_pct DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, instanceName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresTableMaintRow
	for rows.Next() {
		var r PostgresTableMaintRow
		if err := rows.Scan(
			&r.CaptureTimestamp, &r.ServerInstanceName,
			&r.SchemaName, &r.TableName,
			&r.TotalBytes, &r.LiveTuples, &r.DeadTuples, &r.DeadPct,
			&r.SeqScans, &r.IdxScans,
			&r.LastVacuum, &r.LastAutovacuum, &r.LastAnalyze, &r.LastAutoanalyze,
		); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

