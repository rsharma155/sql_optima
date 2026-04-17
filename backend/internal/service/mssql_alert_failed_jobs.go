// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert evaluator – SQL Server failed agent job detection.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// MssqlFailedJobsEvaluator checks for SQL Agent jobs that failed in the last 24 hours.
type MssqlFailedJobsEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewMssqlFailedJobsEvaluator(tsPool *pgxpool.Pool) *MssqlFailedJobsEvaluator {
	return &MssqlFailedJobsEvaluator{tsPool: tsPool}
}

func (e *MssqlFailedJobsEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *MssqlFailedJobsEvaluator) Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT COALESCE(failed_jobs_24h, 0)
		FROM sqlserver_job_metrics
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1`

	var failedJobs int
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&failedJobs); err != nil {
		if isNoDataError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("mssql_failed_jobs: %w", err)
	}
	if failedJobs == 0 {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if failedJobs >= 5 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName:     "mssql_failed_jobs",
		Category:     "jobs",
		Severity:     sev,
		Title:        fmt.Sprintf("SQL Server: %d failed jobs in last 24h", failedJobs),
		Description:  fmt.Sprintf("%d SQL Agent jobs failed in the last 24 hours on %s", failedJobs, instanceName),
		InstanceName: instanceName,
		Engine:       alerts.EngineSQLServer,
		Evidence:     map[string]interface{}{"failed_jobs_24h": failedJobs},
	}}, nil
}
