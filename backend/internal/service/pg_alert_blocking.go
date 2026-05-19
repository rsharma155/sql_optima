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
	serverName := serverID.String()

	// 1. Check active incidents
	q := `
		SELECT incident_id, started_at, root_blocker_pid, COALESCE(root_blocker_query, ''), peak_blocked_sessions
		FROM monitor.pg_blocking_incident
		WHERE server_id = $1 AND status = 'active'
		ORDER BY started_at DESC
		LIMIT 1
	`
	var id int64
	var startedAt time.Time
	var rootPID *int
	var rootQuery string
	var peakVictims int

	err := e.tsPool.QueryRow(ctx, q, serverID).Scan(&id, &startedAt, &rootPID, &rootQuery, &peakVictims)
	if err == nil {
		duration := time.Since(startedAt)
		sev := alerts.SeverityWarning
		if duration > 5*time.Minute || peakVictims > 10 {
			sev = alerts.SeverityCritical
		}

		results = append(results, AlertEvaluatorResult{
			RuleName:    "PGBlockingIncident",
			Category:    "Performance",
			Severity:    sev,
			Title:       fmt.Sprintf("Active PostgreSQL blocking incident (%d victims)", peakVictims),
			Description: fmt.Sprintf("Blocking incident active for %s. Root blocker PID: %v. Query: %s", duration.Round(time.Second), rootPID, rootQuery),
			Evidence: map[string]interface{}{
				"incident_id":           id,
				"started_at":            startedAt,
				"duration_sec":          duration.Seconds(),
				"root_blocker_pid":      rootPID,
				"peak_blocked_sessions": peakVictims,
			},
			ServerID:   serverID,
			ServerName: serverName,
			Engine:     alerts.EnginePostgres,
		})
	}

	// 2. Check for "Idle in Transaction" sessions older than 5 minutes
	qIdle := `
		SELECT count(*)
		FROM monitor.pg_session_snapshot
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		  AND state = 'idle in transaction'
		  AND state_change <= now() - interval '5 minutes'
	`
	var idleCount int
	_ = e.tsPool.QueryRow(ctx, qIdle, serverID).Scan(&idleCount)
	if idleCount > 0 {
		results = append(results, AlertEvaluatorResult{
			RuleName:    "PGIdleInTransaction",
			Category:    "Performance",
			Severity:    alerts.SeverityWarning,
			Title:       fmt.Sprintf("%d sessions idle in transaction > 5m", idleCount),
			Description: "Sessions holding transactions open while idle can cause bloat and prevent vacuum progress.",
			Evidence: map[string]interface{}{
				"idle_session_count": idleCount,
			},
			ServerID:   serverID,
			ServerName: serverName,
			Engine:     alerts.EnginePostgres,
		})
	}

	return results, nil
}
