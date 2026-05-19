// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for SQL Server Agent failed jobs.
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

type SqlServerFailedJobsEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewSqlServerFailedJobsEvaluator(tsPool *pgxpool.Pool) *SqlServerFailedJobsEvaluator {
	return &SqlServerFailedJobsEvaluator{tsPool: tsPool}
}

func (e *SqlServerFailedJobsEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *SqlServerFailedJobsEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT job_name, step_name, error_message, capture_timestamp
		FROM sqlserver_job_failures
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '1 hour'
		ORDER BY capture_timestamp DESC
	`
	rows, err := e.tsPool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlertEvaluatorResult
	serverName := serverID.String()

	seenJobs := make(map[string]bool)
	for rows.Next() {
		var job, step, msg string
		var ts time.Time
		if err := rows.Scan(&job, &step, &msg, &ts); err != nil {
			continue
		}
		if seenJobs[job] {
			continue
		}
		seenJobs[job] = true

		results = append(results, AlertEvaluatorResult{
			RuleName:    "MSJobFailed",
			Category:    "Job Agent",
			Severity:    alerts.SeverityWarning,
			Title:       fmt.Sprintf("SQL Agent Job Failed: %s", job),
			Description: fmt.Sprintf("Job '%s' (step: %s) failed at %s. Error: %s", job, step, ts.Format(time.RFC1123), msg),
			Evidence: map[string]interface{}{
				"job_name":      job,
				"step_name":     step,
				"error_message": msg,
				"failed_at":     ts,
			},
			ServerID:   serverID,
			ServerName: serverName,
			Engine:     alerts.EngineSQLServer,
		})
	}

	return results, nil
}
