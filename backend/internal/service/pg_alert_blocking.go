// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for PostgreSQL blocking sessions and locking incidents.
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

type PgBlockingEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgBlockingEvaluator(tsPool *pgxpool.Pool) *PgBlockingEvaluator {
	return &PgBlockingEvaluator{tsPool: tsPool}
}

func (e *PgBlockingEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgBlockingEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	var results []AlertEvaluatorResult

	if r, ok, err := e.evalBlockingIncident(ctx, serverID); err != nil {
		return nil, err
	} else if ok {
		results = append(results, r)
	}

	if r, ok, err := e.evalIdleInTransaction(ctx, serverID); err != nil {
		return nil, err
	} else if ok {
		results = append(results, r)
	}

	return results, nil
}

func (e *PgBlockingEvaluator) evalBlockingIncident(ctx context.Context, serverID uuid.UUID) (AlertEvaluatorResult, bool, error) {
	q := `
		SELECT incident_id, started_at, ended_at, status, root_blocker_pid,
		       COALESCE(root_blocker_query, ''), COALESCE(peak_blocked_sessions, 0)
		FROM monitor.pg_blocking_incident
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
	if duration > 5*time.Minute || peakVictims > 10 {
		sev = alerts.SeverityCritical
	}

	title := fmt.Sprintf("Active PostgreSQL blocking incident (%d victims)", peakVictims)
	desc := fmt.Sprintf("Blocking incident active for %s. Root blocker PID: %v. Query: %s", duration.Round(time.Second), rootPID, rootQuery)
	if status == "resolved" {
		title = fmt.Sprintf("Recent PostgreSQL blocking incident (%d victims)", peakVictims)
		desc = fmt.Sprintf("Blocking ended %s ago after %s. Peak victims: %d.",
			time.Since(*endedAt).Round(time.Second), duration.Round(time.Second), peakVictims)
	}

	return AlertEvaluatorResult{
		RuleName:    "PGBlockingIncident",
		Category:    "Performance",
		Severity:    sev,
		Title:       title,
		Description: desc,
		Evidence: map[string]interface{}{
			"incident_id":           id,
			"started_at":            startedAt,
			"duration_sec":          duration.Seconds(),
			"root_blocker_pid":      rootPID,
			"peak_blocked_sessions": peakVictims,
			"status":                status,
		},
		ServerID: serverID,
		Engine:   alerts.EnginePostgres,
	}, true, nil
}

func (e *PgBlockingEvaluator) evalIdleInTransaction(ctx context.Context, serverID uuid.UUID) (AlertEvaluatorResult, bool, error) {
	qIdle := `
		SELECT count(*)
		FROM monitor.pg_session_snapshot
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		  AND state = 'idle in transaction'
		  AND state_change <= now() - interval '5 minutes'
	`
	var idleCount int
	err := e.tsPool.QueryRow(ctx, qIdle, serverID).Scan(&idleCount)
	if err != nil || idleCount == 0 {
		return AlertEvaluatorResult{}, false, err
	}

	return AlertEvaluatorResult{
		RuleName:    "PGIdleInTransaction",
		Category:    "Performance",
		Severity:    alerts.SeverityWarning,
		Title:       fmt.Sprintf("%d sessions idle in transaction > 5m", idleCount),
		Description: "Sessions holding transactions open while idle can cause bloat and prevent vacuum progress.",
		Evidence: map[string]interface{}{
			"idle_session_count": idleCount,
		},
		ServerID: serverID,
		Engine:   alerts.EnginePostgres,
	}, true, nil
}
