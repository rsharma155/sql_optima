// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL table maintenance statistics logger (bloat, tuples, vacuum).
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

type PostgresTableMaintRow struct {
	DatabaseName    string     `json:"database_name"`
	SchemaName      string     `json:"schema_name"`
	TableName       string     `json:"table_name"`
	TotalBytes      int64      `json:"total_bytes"`
	LiveTuples      int64      `json:"live_tuples"`
	DeadTuples      int64      `json:"dead_tuples"`
	DeadPct         float64    `json:"dead_pct"`
	SeqScans        int64      `json:"seq_scans"`
	IdxScans        int64      `json:"idx_scans"`
	LastVacuum      *time.Time `json:"last_vacuum"`
	LastAutovacuum  *time.Time `json:"last_autovacuum"`
	LastAnalyze     *time.Time `json:"last_analyze"`
	LastAutoanalyze *time.Time `json:"last_autoanalyze"`
}

type PostgresTableMaintResponse struct {
	CaptureTimestamp time.Time  `json:"capture_timestamp"`
	ServerID         uuid.UUID  `json:"server_id"`
	DatabaseName     string     `json:"database_name"`
	SchemaName       string     `json:"schema_name"`
	TableName        string     `json:"table_name"`
	TotalBytes       int64      `json:"total_bytes"`
	LiveTuples       int64      `json:"live_tuples"`
	DeadTuples       int64      `json:"dead_tuples"`
	DeadPct          float64    `json:"dead_pct"`
	SeqScans         int64      `json:"seq_scans"`
	IdxScans         int64      `json:"idx_scans"`
	LastVacuum       *time.Time `json:"last_vacuum"`
	LastAutovacuum   *time.Time `json:"last_autovacuum"`
	LastAnalyze      *time.Time `json:"last_analyze"`
	LastAutoanalyze  *time.Time `json:"last_autoanalyze"`
}

func (tl *TimescaleLogger) LogPostgresTableMaintStats(ctx context.Context, serverID uuid.UUID, rows []PostgresTableMaintRow) error {
	if len(rows) == 0 {
		return nil
	}
	now := time.Now().UTC()

	sig := pgFnv64(serverID, len(rows))
	for _, r := range rows {
		sig = pgFnv64(sig, r.DatabaseName, r.SchemaName, r.TableName, r.TotalBytes, r.LiveTuples, r.DeadTuples)
	}

	key := "pg_tblmaint|" + serverID.String()
	tl.mu.Lock()
	if prev, ok := tl.prevPgTblMaintHash[serverID]; ok && prev == sig {
		tl.mu.Unlock()
		return nil
	}
	tl.prevPgTblMaintHash[serverID] = sig
	tl.mu.Unlock()

	q := `
		INSERT INTO postgres_table_maintenance_stats (
			capture_timestamp, server_id, database_name, schema_name, table_name,
			total_bytes, live_tuples, dead_tuples, dead_pct, seq_scans, idx_scans,
			last_vacuum, last_autovacuum, last_analyze, last_autoanalyze
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, r.DatabaseName, r.SchemaName, r.TableName,
			r.TotalBytes, r.LiveTuples, r.DeadTuples, r.DeadPct, r.SeqScans, r.IdxScans,
			r.LastVacuum, r.LastAutovacuum, r.LastAnalyze, r.LastAutoanalyze)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	_ = key
	return nil
}

func (tl *TimescaleLogger) GetPostgresTableMaintHistory(ctx context.Context, serverID uuid.UUID, database, schema, table string, limit int) ([]PostgresTableMaintResponse, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
		SELECT capture_timestamp, server_id, database_name, schema_name, table_name,
		       total_bytes, live_tuples, dead_tuples, dead_pct, seq_scans, idx_scans,
		       last_vacuum, last_autovacuum, last_analyze, last_autoanalyze
		FROM postgres_table_maintenance_stats
		WHERE server_id = $1 AND database_name = $2 AND schema_name = $3 AND table_name = $4
		ORDER BY capture_timestamp DESC
		LIMIT $5
	`
	rows, err := tl.pool.Query(ctx, q, serverID, database, schema, table, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresTableMaintResponse
	for rows.Next() {
		var r PostgresTableMaintResponse
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.DatabaseName, &r.SchemaName, &r.TableName,
			&r.TotalBytes, &r.LiveTuples, &r.DeadTuples, &r.DeadPct, &r.SeqScans, &r.IdxScans,
			&r.LastVacuum, &r.LastAutovacuum, &r.LastAnalyze, &r.LastAutoanalyze); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (tl *TimescaleLogger) GetLatestPostgresTableMaint(ctx context.Context, serverID uuid.UUID, limit int) ([]PostgresTableMaintResponse, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `
		WITH latest AS (
			SELECT DISTINCT ON (database_name, schema_name, table_name)
				capture_timestamp, server_id, database_name, schema_name, table_name,
				total_bytes, live_tuples, dead_tuples, dead_pct, seq_scans, idx_scans,
				last_vacuum, last_autovacuum, last_analyze, last_autoanalyze
			FROM postgres_table_maintenance_stats
			WHERE server_id = $1
			ORDER BY database_name, schema_name, table_name, capture_timestamp DESC
		)
		SELECT * FROM latest
		ORDER BY total_bytes DESC
		LIMIT $2
	`
	rows, err := tl.pool.Query(ctx, q, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PostgresTableMaintResponse
	for rows.Next() {
		var r PostgresTableMaintResponse
		if err := rows.Scan(&r.CaptureTimestamp, &r.ServerID, &r.DatabaseName, &r.SchemaName, &r.TableName,
			&r.TotalBytes, &r.LiveTuples, &r.DeadTuples, &r.DeadPct, &r.SeqScans, &r.IdxScans,
			&r.LastVacuum, &r.LastAutovacuum, &r.LastAnalyze, &r.LastAutoanalyze); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
