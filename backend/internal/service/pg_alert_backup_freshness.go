// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for PostgreSQL backup freshness.
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

type PgBackupFreshnessEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgBackupFreshnessEvaluator(tsPool *pgxpool.Pool) *PgBackupFreshnessEvaluator {
	return &PgBackupFreshnessEvaluator{tsPool: tsPool}
}

func (e *PgBackupFreshnessEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgBackupFreshnessEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	// 1. Fetch latest successful backup run for this serverID
	q := `
		SELECT capture_timestamp, status, finished_at, backup_type
		FROM postgres_backup_runs
		WHERE server_id = $1 AND lower(status) IN ('completed', 'success', 'succeeded', 'ok')
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var lastTs time.Time
	var status, bType string
	var endTime *time.Time
	err := e.tsPool.QueryRow(ctx, q, serverID).Scan(&lastTs, &status, &endTime, &bType)

	var results []AlertEvaluatorResult

	// Default server name if we can't find it
	serverName := serverID.String()

	if err != nil {
		// No successful backup found
		results = append(results, AlertEvaluatorResult{
			RuleName:    "PGBackupNeverRun",
			Category:    "Backup",
			Severity:    alerts.SeverityCritical,
			Title:       "No successful PostgreSQL backups found",
			Description: "No completed backup records were found in the monitoring history for this instance.",
			ServerID:    serverID,
			ServerName:  serverName,
			Engine:      alerts.EnginePostgres,
		})
		return results, nil
	}

	maxAgeHours := 24
	var policyHours int
	if err := e.tsPool.QueryRow(ctx, `
		SELECT rpo_backup_hours FROM optima_server_dr_policy WHERE server_id = $1`, serverID).Scan(&policyHours); err == nil && policyHours > 0 {
		maxAgeHours = policyHours
	}
	limit := time.Duration(maxAgeHours) * time.Hour

	// 2. Check age against per-instance DR policy
	age := time.Since(lastTs)
	if age > limit {
		sev := alerts.SeverityWarning
		if age > limit*2 {
			sev = alerts.SeverityCritical
		}

		results = append(results, AlertEvaluatorResult{
			RuleName:    "PGBackupStale",
			Category:    "Backup",
			Severity:    sev,
			Title:       fmt.Sprintf("PostgreSQL backup is stale (%s)", age.Round(time.Hour)),
			Description: fmt.Sprintf("The last successful %s backup was completed at %s.", bType, lastTs.Format(time.RFC1123)),
			Evidence: map[string]interface{}{
				"last_success": lastTs,
				"age_hours":    age.Hours(),
				"backup_type":  bType,
			},
			ServerID:   serverID,
			ServerName: serverName,
			Engine:     alerts.EnginePostgres,
		})
	}

	return results, nil
}
