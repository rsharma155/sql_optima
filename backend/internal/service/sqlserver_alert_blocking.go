// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for SQL Server blocking sessions.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

type SqlServerBlockingEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewSqlServerBlockingEvaluator(tsPool *pgxpool.Pool) *SqlServerBlockingEvaluator {
	return &SqlServerBlockingEvaluator{tsPool: tsPool}
}

func (e *SqlServerBlockingEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *SqlServerBlockingEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	if r, ok, err := e.evalFromIncident(ctx, serverID); err != nil {
		return nil, err
	} else if ok {
		return []AlertEvaluatorResult{r}, nil
	}
	if r, ok, err := e.evalFromLiveSnapshot(ctx, serverID); err != nil {
		return nil, err
	} else if ok {
		return []AlertEvaluatorResult{r}, nil
	}
	return nil, nil
}

func (e *SqlServerBlockingEvaluator) evalFromIncident(ctx context.Context, serverID uuid.UUID) (AlertEvaluatorResult, bool, error) {
	q := `
		SELECT incident_id, started_at, ended_at, status, root_blocker_pid,
		       COALESCE(root_blocker_query, ''), COALESCE(peak_blocked_sessions, 0)
		FROM sqlserver_blocking_incidents
		WHERE server_id = $1
		  AND (
		    status = 'active'
		    OR (status = 'resolved' AND ended_at >= now() - interval '15 minutes' AND COALESCE(peak_blocked_sessions, 0) > 0)
		  )
		ORDER BY started_at DESC
		LIMIT 1
	`
	var id int64
	var startedAt time.Time
	var endedAt *time.Time
	var status string
	var rootPID *int
	var rootQuery string
	var peakVictims int

	err := e.tsPool.QueryRow(ctx, q, serverID).Scan(&id, &startedAt, &endedAt, &status, &rootPID, &rootQuery, &peakVictims)
	if err == pgx.ErrNoRows {
		return AlertEvaluatorResult{}, false, nil
	}
	if err != nil {
		return AlertEvaluatorResult{}, false, err
	}

	duration := time.Since(startedAt)
	if status == "resolved" && endedAt != nil {
		duration = endedAt.Sub(startedAt)
	}
	sev := alerts.SeverityWarning
	if duration > 5*time.Minute || peakVictims > 5 {
		sev = alerts.SeverityCritical
	}

	title := fmt.Sprintf("Active SQL Server blocking incident (%d victims)", peakVictims)
	desc := fmt.Sprintf("Blocking incident active for %s. Root blocker PID: %v.", duration.Round(time.Second), rootPID)
	if status == "resolved" {
		title = fmt.Sprintf("Recent SQL Server blocking incident (%d victims)", peakVictims)
		desc = fmt.Sprintf("Blocking ended %s ago after %s. Peak victims: %d. Root blocker PID: %v.",
			time.Since(*endedAt).Round(time.Second), duration.Round(time.Second), peakVictims, rootPID)
	}

	return AlertEvaluatorResult{
		RuleName:    "MSBlockingIncident",
		Category:    "Performance",
		Severity:    sev,
		Title:       title,
		Description: desc,
		Evidence: map[string]interface{}{
			"incident_id":           id,
			"started_at":            startedAt,
			"duration_sec":          duration.Seconds(),
			"peak_blocked_sessions": peakVictims,
			"status":                status,
		},
		ServerID: serverID,
		Engine:   alerts.EngineSQLServer,
	}, true, nil
}

func (e *SqlServerBlockingEvaluator) evalFromLiveSnapshot(ctx context.Context, serverID uuid.UUID) (AlertEvaluatorResult, bool, error) {
	q := `
		WITH latest AS (
			SELECT capture_timestamp
			FROM sqlserver_blocking_snapshots
			WHERE server_id = $1
			ORDER BY capture_timestamp DESC
			LIMIT 1
		),
		current_snaps AS (
			SELECT *
			FROM sqlserver_blocking_snapshots
			WHERE server_id = $1
			  AND capture_timestamp = (SELECT capture_timestamp FROM latest)
		)
		SELECT
			COALESCE((SELECT COUNT(*) FROM current_snaps WHERE blocking_session_id != 0), 0),
			COALESCE((SELECT MAX(wait_duration_ms) FROM current_snaps WHERE blocking_session_id != 0), 0),
			COALESCE(EXTRACT(EPOCH FROM (NOW() - (SELECT capture_timestamp FROM latest)))::int, -1)
	`
	var blockedCount int
	var maxWaitMs int64
	var snapshotAgeSec int64
	err := e.tsPool.QueryRow(ctx, q, serverID).Scan(&blockedCount, &maxWaitMs, &snapshotAgeSec)
	if err == pgx.ErrNoRows || blockedCount == 0 {
		return AlertEvaluatorResult{}, false, nil
	}
	if err != nil {
		return AlertEvaluatorResult{}, false, err
	}
	// Ignore stale snapshots (collector gap or instance offline).
	if snapshotAgeSec < 0 || snapshotAgeSec > 180 {
		return AlertEvaluatorResult{}, false, nil
	}

	sev := alerts.SeverityWarning
	if blockedCount > 5 || maxWaitMs > 30000 {
		sev = alerts.SeverityCritical
	}

	return AlertEvaluatorResult{
		RuleName: "MSBlockingIncident",
		Category: "Performance",
		Severity: sev,
		Title:    fmt.Sprintf("SQL Server blocking detected (%d blocked sessions)", blockedCount),
		Description: fmt.Sprintf(
			"%d session(s) blocked in the latest collector snapshot (max wait %d ms, snapshot %d s ago).",
			blockedCount, maxWaitMs, snapshotAgeSec,
		),
		Evidence: map[string]interface{}{
			"blocked_sessions":  blockedCount,
			"max_wait_ms":       maxWaitMs,
			"snapshot_age_sec":  snapshotAgeSec,
			"source":            "live_snapshot",
		},
		ServerID: serverID,
		Engine:   alerts.EngineSQLServer,
	}, true, nil
}
