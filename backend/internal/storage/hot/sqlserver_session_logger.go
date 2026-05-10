// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: TimescaleDB storage-layer for SQL Server session snapshots and identity mapping.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package hot

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rsharma155/sql_optima/internal/models"
)

// LogSQLServerSessionSnapshots persists session snapshots to TimescaleDB
func (tl *TimescaleLogger) LogSQLServerSessionSnapshots(ctx context.Context, snapshots []models.SQLServerSessionSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO sqlserver_session_snapshot (
			sample_time, instance_id, session_id, login_name, original_login_name,
			host_name, program_name, database_name, is_user_process, status,
			query_hash, query_plan_hash,
			total_elapsed_time_ms, cpu_time_ms, wait_type, blocking_session_id, query_text
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	for _, s := range snapshots {
		qh := bytesToInt64(s.QueryHash)
		qph := bytesToInt64(s.QueryPlanHash)

		batch.Queue(query,
			s.SampleTime, 
			s.InstanceID, 
			s.SessionID, 
			tl.ToSafeUTF8(s.LoginName), 
			tl.ToSafeUTF8(s.OriginalLoginName),
			tl.ToSafeUTF8(s.HostName), 
			tl.ToSafeUTF8(s.ProgramName), 
			tl.ToSafeUTF8(s.DatabaseName), 
			s.IsUserProcess, 
			tl.ToSafeUTF8(s.Status),
			qh, 
			qph,
			s.TotalElapsedTimeMs,
			s.CPUTimeMs,
			tl.ToSafeUTF8(s.WaitType),
			s.BlockingSessionID,
			tl.ToSafeUTF8(s.QueryText),
		)
	}

	res := tl.pool.SendBatch(ctx, batch)
	if err := res.Close(); err != nil {
		return fmt.Errorf("LogSQLServerSessionSnapshots batch: %w", err)
	}

	return nil
}

func bytesToInt64(b []byte) *int64 {
	if len(b) == 0 {
		return nil
	}
	var val int64
	if len(b) >= 8 {
		val = int64(binary.BigEndian.Uint64(b[len(b)-8:]))
	} else {
		var u uint64
		for _, x := range b {
			u = (u << 8) | uint64(x)
		}
		val = int64(u)
	}
	return &val
}

// RunSQLServerIdentityUpsertJob executes the MERGE logic to map query hashes to identities
func (tl *TimescaleLogger) RunSQLServerIdentityUpsertJob(ctx context.Context) error {
	query := `
INSERT INTO public.sqlserver_query_identity_dim (
    instance_id,
    query_hash,
    database_name,
    login_name,
    host_name,
    program_name,
    first_seen,
    last_seen,
    seen_count
)
SELECT DISTINCT
    instance_id,
    query_hash,
    database_name,
    login_name,
    host_name,
    program_name,
    NOW(),
    NOW(),
    1
FROM public.sqlserver_session_snapshot
WHERE query_hash IS NOT NULL
  AND is_user_process = TRUE
  AND login_name NOT IN (
       'NT AUTHORITY\SYSTEM',
       'NT SERVICE\SQLSERVERAGENT'
  )
  AND sample_time > NOW() - INTERVAL '5 minutes'
ON CONFLICT (instance_id, query_hash, login_name, host_name, program_name)
DO UPDATE SET
    last_seen = NOW(),
    seen_count = public.sqlserver_query_identity_dim.seen_count + 1;
`
	_, err := tl.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("RunSQLServerIdentityUpsertJob: %w", err)
	}

	return nil
}

// RunSQLServerClassificationJob executes the 3-step logic to classify queries as USER, SYSTEM, or UNKNOWN
func (tl *TimescaleLogger) RunSQLServerClassificationJob(ctx context.Context) error {
	// Step 1: Insert all query_hash from query stats as UNKNOWN
	step1 := `
INSERT INTO public.sqlserver_query_classification_dim(instance_id, query_hash, classification, first_seen, last_seen)
SELECT DISTINCT
    instance_id,
    query_hash,
    'UNKNOWN',
    NOW(),
    NOW()
FROM public.sqlserver_query_stats_snapshot_v2
ON CONFLICT (instance_id, query_hash) DO UPDATE
SET last_seen = NOW();
`
	// Step 2: Promote USER queries
	step2 := `
UPDATE public.sqlserver_query_classification_dim c
SET classification = 'USER'
FROM public.sqlserver_query_identity_dim i
WHERE c.instance_id = i.instance_id 
  AND c.query_hash = i.query_hash
  AND c.classification <> 'USER';
`
	// Step 2.5: Mark queries with 'sys.' or '[sys].' as SYSTEM
	step25 := `
UPDATE public.sqlserver_query_classification_dim c
SET classification = 'SYSTEM'
FROM public.sqlserver_query_metrics_v2 q
WHERE c.instance_id = q.instance_id 
  AND c.query_hash = q.query_hash
  AND (
       UPPER(q.statement_text) LIKE '%SYS.%' 
    OR UPPER(q.statement_text) LIKE '%[SYS].%'
    OR UPPER(q.query_text_raw) LIKE '%SYS.%'
    OR UPPER(q.query_text_raw) LIKE '%[SYS].%'
  )
  AND c.classification <> 'SYSTEM';
`
	// Step 3: Mark remaining as SYSTEM if not seen in identity for 1 hour
	step3 := `
UPDATE public.sqlserver_query_classification_dim
SET classification = 'SYSTEM'
WHERE classification = 'UNKNOWN'
  AND last_seen < NOW() - INTERVAL '1 hour';
`

	tx, err := tl.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("RunSQLServerClassificationJob begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, step1); err != nil {
		return fmt.Errorf("RunSQLServerClassificationJob step 1: %w", err)
	}
	if _, err := tx.Exec(ctx, step2); err != nil {
		return fmt.Errorf("RunSQLServerClassificationJob step 2: %w", err)
	}
	if _, err := tx.Exec(ctx, step25); err != nil {
		return fmt.Errorf("RunSQLServerClassificationJob step 2.5: %w", err)
	}
	if _, err := tx.Exec(ctx, step3); err != nil {
		return fmt.Errorf("RunSQLServerClassificationJob step 3: %w", err)
	}

	return tx.Commit(ctx)
}

// GetLatestSQLServerSessionSnapshots fetches the most recent snapshots for an instance
func (tl *TimescaleLogger) GetLatestSQLServerSessionSnapshots(ctx context.Context, instanceID string, dbFilter string) ([]models.SQLServerSessionSnapshot, error) {
	// 1. Find the latest timestamp
	var latestTs *time.Time
	err := tl.pool.QueryRow(ctx, "SELECT MAX(sample_time) FROM sqlserver_session_snapshot WHERE instance_id = $1", instanceID).Scan(&latestTs)
	if err != nil || latestTs == nil {
		return nil, err
	}

	// 2. Fetch all snapshots for that timestamp
	q := `
		SELECT sample_time, instance_id, session_id, login_name, original_login_name,
		       host_name, program_name, database_name, is_user_process, status,
		       query_hash, query_plan_hash,
		       total_elapsed_time_ms, cpu_time_ms, wait_type, blocking_session_id, query_text
		FROM sqlserver_session_snapshot
		WHERE instance_id = $1 AND sample_time = $2
	`
	args := []interface{}{instanceID, *latestTs}
	if dbFilter != "" {
		q += " AND database_name = $3"
		args = append(args, dbFilter)
	}

	rows, err := tl.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.SQLServerSessionSnapshot
	for rows.Next() {
		var s models.SQLServerSessionSnapshot
		var qh, qph *int64
		if err := rows.Scan(
			&s.SampleTime, &s.InstanceID, &s.SessionID, &s.LoginName, &s.OriginalLoginName,
			&s.HostName, &s.ProgramName, &s.DatabaseName, &s.IsUserProcess, &s.Status,
			&qh, &qph,
			&s.TotalElapsedTimeMs, &s.CPUTimeMs, &s.WaitType, &s.BlockingSessionID, &s.QueryText,
		); err != nil {
			continue
		}
		// Convert int64 back to []byte for compatibility if needed, 
		// but frontend mostly uses query_hash as string/hex which we handle elsewhere.
		results = append(results, s)
	}
	return results, nil
}
