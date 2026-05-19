// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB writer for the collector metrics pipeline.
//          Handles both SQL Server and PostgreSQL metrics ingestion.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package timescaledb

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/collectors/domain"
)

type TimescaleWriter struct {
	pool *pgxpool.Pool
}

func NewTimescaleWriter(pool *pgxpool.Pool) *TimescaleWriter {
	return &TimescaleWriter{pool: pool}
}

func (w *TimescaleWriter) WriteMSSQLMetrics(ctx context.Context, metrics []domain.MSSQLCombinedMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	const q = `INSERT INTO sqlserver_query_metrics_v2 (
		capture_timestamp, server_id, database_name, login_name, application_name,
		query_hash, plan_hash, total_executions, total_cpu_ms, total_elapsed_ms,
		total_logical_reads, total_physical_reads, total_rows, statement_text, query_text_raw,
		last_execution_time, is_user_workload
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	batch := &pgx.Batch{}
	for _, m := range metrics {
		batch.Queue(q,
			m.Timestamp, m.ServerID, m.DatabaseName, m.LoginName, m.ApplicationName,
			m.QueryHash, m.PlanHash, m.TotalExecutions, m.TotalCPUMs, m.TotalElapsedMs,
			m.TotalLogicalReads, m.TotalPhysicalReads, m.TotalRows, m.StatementText, m.QueryTextRaw,
			m.LastExecutionTime, m.IsUserWorkload,
		)
	}

	br := w.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range metrics {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (w *TimescaleWriter) WritePGMetrics(ctx context.Context, metrics []domain.PGCombinedMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	const q = `INSERT INTO pg_query_metrics_v2 (
		capture_timestamp, server_id, datname, usename, application_name,
		queryid, query, calls, total_exec_time, rows, shared_blks_hit,
		shared_blks_read, temp_blks_written
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	batch := &pgx.Batch{}
	for _, m := range metrics {
		batch.Queue(q,
			m.Timestamp, m.ServerID, m.DatabaseName, m.UserName, m.ApplicationName,
			m.QueryID, m.Query, m.Calls, m.TotalExecTime, m.Rows, m.SharedBlksHit,
			m.SharedBlksRead, m.TempBlksWritten,
		)
	}

	br := w.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range metrics {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}
