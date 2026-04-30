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
			query_hash, query_plan_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	for _, s := range snapshots {
		batch.Queue(query,
			s.SampleTime, s.InstanceID, s.SessionID, s.LoginName, s.OriginalLoginName,
			s.HostName, s.ProgramName, s.DatabaseName, s.IsUserProcess, s.Status,
			s.QueryHash, s.QueryPlanHash,
		)
	}

	res := tl.pool.SendBatch(ctx, batch)
	if err := res.Close(); err != nil {
		return fmt.Errorf("LogSQLServerSessionSnapshots batch: %w", err)
	}

	return nil
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
		       query_hash, query_plan_hash
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
		if err := rows.Scan(
			&s.SampleTime, &s.InstanceID, &s.SessionID, &s.LoginName, &s.OriginalLoginName,
			&s.HostName, &s.ProgramName, &s.DatabaseName, &s.IsUserProcess, &s.Status,
			&s.QueryHash, &s.QueryPlanHash,
		); err != nil {
			continue
		}
		results = append(results, s)
	}
	return results, nil
}
