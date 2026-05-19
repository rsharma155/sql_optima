// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage for pulse metrics and staging snapshots.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"log/slog"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (tl *TimescaleLogger) LogCollectorRun(ctx context.Context, serverID uuid.UUID, engine string, status string, durationMs int64) error {
	q := `INSERT INTO monitor.collector_run_log (capture_timestamp, server_id, engine, status, duration_ms) VALUES (now(), $1, $2, $3, $4)`
	_, err := tl.pool.Exec(ctx, q, serverID, engine, status, durationMs)
	return err
}

func (tl *TimescaleLogger) LogStagingTier1SQLServer(ctx context.Context, serverID uuid.UUID, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	q := `INSERT INTO staging.sqlserver_session_request_raw (
		capture_timestamp, server_id, session_id, db_name, login_name, host_name, program_name, 
		request_status, wait_type, wait_time, last_wait_type, cpu_time_ms, elapsed_time_ms, 
		reads, writes, granted_query_memory, row_count, percent_complete, command
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`
	
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, 
			now, serverID, getInt(r, "session_id"), getStr(r, "database_name"), getStr(r, "login_name"), 
			getStr(r, "host_name"), getStr(r, "program_name"), getStr(r, "status"), getStr(r, "wait_type"), 
			getInt(r, "wait_time"), getStr(r, "last_wait_type"), getInt(r, "cpu_time"), 
			getInt(r, "total_elapsed_time"), getInt64FromMap(r, "reads"), getInt64FromMap(r, "writes"), 
			getInt64FromMap(r, "granted_memory_mb"), getInt64FromMap(r, "row_count"), 
			parsePercentToFloat(getStr(r, "percent_complete")), getStr(r, "command"),
		)
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			slog.Error("[TSLogger] SQL Server T1 batch insert failed", "err", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogStagingTier1SQLServerLocks(ctx context.Context, serverID uuid.UUID, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	// Use the correct staging table for raw session info if it's about blocking
	q := `INSERT INTO staging.sqlserver_session_request_raw (
		capture_timestamp, server_id, session_id, wait_time, wait_type, blocking_session_id
	) VALUES ($1, $2, $3, $4, $5, $6)`
	
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, getInt(r, "session_id"), getInt(r, "wait_duration_ms"), getStr(r, "wait_type"), getInt(r, "blocking_session_id"))
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			slog.Error("[TSLogger] SQL Server T1 Locks batch insert failed", "err", err)
		}
	}
	return nil
}


func (tl *TimescaleLogger) LogStagingTier2SQLServer(ctx context.Context, serverID uuid.UUID, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	// Mapping to staging.sqlserver_perf_system_raw as a fallback for Tier 2 metrics
	q := `INSERT INTO staging.sqlserver_perf_system_raw (capture_timestamp, server_id, category, metric_name, metric_value, unit, instance_name)
	      VALUES ($1, $2, $3, $4, $5, $6, $7)`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, "Database", "SizeMB", int64(getFloat64(r, "total_size_mb")), "MB", getStr(r, "database_name"))
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			slog.Error("[TSLogger] SQL Server T2 batch insert failed", "err", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogStagingTier3SQLServerIO(ctx context.Context, serverID uuid.UUID, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	q := `INSERT INTO staging.sqlserver_io_raw (capture_timestamp, server_id, db_name, file_id, io_stall_read_ms, io_stall_write_ms)
	      VALUES ($1, $2, $3, $4, $5, $6)`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, getStr(r, "database_name"), getInt(r, "file_id"), int64(getFloat64(r, "read_latency_ms")), int64(getFloat64(r, "write_latency_ms")))
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			slog.Error("[TSLogger] SQL Server IO batch insert failed", "err", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogStagingTier3SQLServerMemoryGrants(ctx context.Context, serverID uuid.UUID, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	// Mapping to the main sqlserver_memory_grants table as there is no staging for it
	q := `INSERT INTO public.sqlserver_memory_grants (capture_timestamp, server_id, active_grants, granted_memory_mb)
	      VALUES ($1, $2, $3, $4)`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		// Summarize into the expected schema if possible, or skip if we can't map accurately
		b.Queue(q, now, serverID, getInt64FromMap(r, "active_grants"), getFloat64(r, "granted_memory_mb"))
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for range rows {
		if _, err := br.Exec(); err != nil {
			slog.Error("[TSLogger] SQL Server Memory Grants batch insert failed", "err", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) LogStagingTier1Postgres(ctx context.Context, serverID uuid.UUID, rows []map[string]interface{}) error {
	if len(rows) == 0 {
		return nil
	}
	q := `INSERT INTO staging.postgres_activity_locks_raw (capture_timestamp, server_id, pid, state, wait_event, query)
	      VALUES ($1, $2, $3, $4, $5, $6)`
	now := time.Now().UTC()
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(q, now, serverID, getInt(r, "pid"), getStr(r, "state"), getStr(r, "wait_event"), getStr(r, "query"))
	}
	br := tl.pool.SendBatch(ctx, b)
	defer br.Close()
	for i := 0; i < len(rows); i++ {
		if _, err := br.Exec(); err != nil {
			slog.Error("[TSLogger] Postgres T1 batch insert failed", "err", err)
		}
	}
	return nil
}
