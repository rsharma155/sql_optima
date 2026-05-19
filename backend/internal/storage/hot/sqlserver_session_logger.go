// SQL Optima — https://github.com/rsharma155/sql_optima
//
// File: sqlserver_session_logger.go
// Purpose: Persistence layer for SQL Server session snapshots in TimescaleDB.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package hot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/models"
)

// LogSQLServerSessionSnapshot persists a batch of session snapshots.
func (tl *TimescaleLogger) LogSQLServerSessionSnapshot(ctx context.Context, serverID uuid.UUID, snapshots []models.SQLServerSessionSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	q := `
		INSERT INTO sqlserver_session_snapshots (
			capture_timestamp, server_id, session_id, login_name, original_login_name,
			host_name, database_name, program_name, status, is_user_process,
			cpu_time_ms, total_elapsed_time_ms, memory_usage_pages, reads, writes,
			logical_reads, open_transaction_count, wait_type, wait_time_ms,
			last_wait_type, wait_resource, blocking_session_id, query_hash,
			query_plan_hash, query_text
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
	`

	batch := &pgx.Batch{}
	for _, s := range snapshots {
		var qh, qph int64
		if h, err := strconv.ParseUint(strings.TrimPrefix(s.QueryHash, "0x"), 16, 64); err == nil {
			qh = int64(h)
		}
		if h, err := strconv.ParseUint(strings.TrimPrefix(s.QueryPlanHash, "0x"), 16, 64); err == nil {
			qph = int64(h)
		}

		batch.Queue(q,
			s.CaptureTimestamp, serverID, s.SessionID, s.LoginName, s.OriginalLoginName,
			s.HostName, s.DatabaseName, s.ProgramName, s.Status, s.IsUserProcess,
			s.CPUTimeMs, s.TotalElapsedTimeMs, s.MemoryUsagePages, s.Reads, s.Writes,
			s.LogicalReads, s.OpenTransactionCount, s.WaitType, s.WaitTimeMs,
			s.LastWaitType, s.WaitResource, s.BlockingSessionID, qh, qph, s.QueryText,
		)
	}

	br := tl.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range snapshots {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("LogSQLServerSessionSnapshot batch exec: %w", err)
		}
	}
	return nil
}

func (tl *TimescaleLogger) GetLatestSQLServerSessions(ctx context.Context, serverID uuid.UUID) ([]models.SQLServerSessionSnapshot, error) {
	q := `
		SELECT 
			capture_timestamp, server_id, session_id, login_name, original_login_name,
			host_name, database_name, program_name, status, is_user_process,
			cpu_time_ms, total_elapsed_time_ms, memory_usage_pages, reads, writes,
			logical_reads, open_transaction_count, wait_type, wait_time_ms,
			last_wait_type, wait_resource, blocking_session_id, query_hash,
			query_plan_hash, query_text
		FROM sqlserver_session_snapshots
		WHERE server_id = $1
		  AND capture_timestamp = (SELECT MAX(capture_timestamp) FROM sqlserver_session_snapshots WHERE server_id = $1)
	`
	rows, err := tl.pool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerSessionSnapshot
	for rows.Next() {
		var s models.SQLServerSessionSnapshot
		var qh, qph int64
		err := rows.Scan(
			&s.CaptureTimestamp, &s.ServerID, &s.SessionID, &s.LoginName, &s.OriginalLoginName,
			&s.HostName, &s.DatabaseName, &s.ProgramName, &s.Status, &s.IsUserProcess,
			&s.CPUTimeMs, &s.TotalElapsedTimeMs, &s.MemoryUsagePages, &s.Reads, &s.Writes,
			&s.LogicalReads, &s.OpenTransactionCount, &s.WaitType, &s.WaitTimeMs,
			&s.LastWaitType, &s.WaitResource, &s.BlockingSessionID, &qh, &qph, &s.QueryText,
		)
		if err != nil {
			continue
		}
		s.QueryHash = fmt.Sprintf("0x%X", uint64(qh))
		s.QueryPlanHash = fmt.Sprintf("0x%X", uint64(qph))
		s.SampleTime = s.CaptureTimestamp
		results = append(results, s)
	}
	return results, rows.Err()
}

// RunSQLServerIdentityUpsertJob is a stub for the identity upsert job.
func (tl *TimescaleLogger) RunSQLServerIdentityUpsertJob(ctx context.Context) error {
	return nil
}

// RunSQLServerClassificationJob is a stub for the classification job.
func (tl *TimescaleLogger) RunSQLServerClassificationJob(ctx context.Context) error {
	return nil
}
