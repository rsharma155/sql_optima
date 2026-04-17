// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert evaluator – PostgreSQL backup freshness detection.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

// PgBackupFreshnessEvaluator checks for stale PostgreSQL backups.
type PgBackupFreshnessEvaluator struct {
	tsPool      *pgxpool.Pool
	maxAgeHours int
}

func NewPgBackupFreshnessEvaluator(tsPool *pgxpool.Pool) *PgBackupFreshnessEvaluator {
	return &PgBackupFreshnessEvaluator{
		tsPool:      tsPool,
		maxAgeHours: 24,
	}
}

func (e *PgBackupFreshnessEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgBackupFreshnessEvaluator) Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT finished_at, status, tool, backup_type
		FROM postgres_backup_runs
		WHERE server_instance_name = $1
		  AND status = 'success'
		ORDER BY capture_timestamp DESC
		LIMIT 1`

	var finishedAt time.Time
	var status, tool, backupType string
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&finishedAt, &status, &tool, &backupType); err != nil {
		if isNoDataError(err) {
			// No backup records at all — emit a warning
			return []AlertEvaluatorResult{{
				RuleName:     "pg_backup_freshness",
				Category:     "backup",
				Severity:     alerts.SeverityWarning,
				Title:        "PostgreSQL: no backup records found",
				Description:  fmt.Sprintf("No successful backup records found for %s", instanceName),
				InstanceName: instanceName,
				Engine:       alerts.EnginePostgres,
				Evidence:     map[string]interface{}{"reason": "no_backup_records"},
			}}, nil
		}
		return nil, fmt.Errorf("pg_backup_freshness query: %w", err)
	}

	ageHours := time.Since(finishedAt).Hours()
	if ageHours <= float64(e.maxAgeHours) {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if ageHours >= float64(e.maxAgeHours)*2 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName:     "pg_backup_freshness",
		Category:     "backup",
		Severity:     sev,
		Title:        fmt.Sprintf("PostgreSQL backup stale: %.0fh since last successful backup", ageHours),
		Description:  fmt.Sprintf("Last successful %s backup (%s) for %s was %.0f hours ago (threshold: %dh)", backupType, tool, instanceName, ageHours, e.maxAgeHours),
		InstanceName: instanceName,
		Engine:       alerts.EnginePostgres,
		Evidence: map[string]interface{}{
			"last_backup_at": finishedAt,
			"age_hours":      ageHours,
			"max_age_hours":  e.maxAgeHours,
			"tool":           tool,
			"backup_type":    backupType,
		},
	}}, nil
}
