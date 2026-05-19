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
	var results []AlertEvaluatorResult
	serverName := serverID.String()

	q := `
		SELECT incident_id, started_at, root_blocker_pid, COALESCE(root_blocker_query, ''), peak_blocked_sessions
		FROM sqlserver_blocking_incidents
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
		if duration > 5*time.Minute || peakVictims > 5 {
			sev = alerts.SeverityCritical
		}

		results = append(results, AlertEvaluatorResult{
			RuleName:    "MSBlockingIncident",
			Category:    "Performance",
			Severity:    sev,
			Title:       fmt.Sprintf("Active SQL Server blocking incident (%d victims)", peakVictims),
			Description: fmt.Sprintf("Blocking incident active for %s. Root blocker PID: %v.", duration.Round(time.Second), rootPID),
			Evidence: map[string]interface{}{
				"incident_id":           id,
				"started_at":            startedAt,
				"duration_sec":          duration.Seconds(),
				"peak_blocked_sessions": peakVictims,
			},
			ServerID:   serverID,
			ServerName: serverName,
			Engine:     alerts.EngineSQLServer,
		})
	}

	return results, nil
}
