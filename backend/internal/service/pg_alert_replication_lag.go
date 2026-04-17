// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert evaluator – PostgreSQL replication lag threshold detection.
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

// PgReplicationLagEvaluator checks for PostgreSQL replication lag exceeding threshold.
type PgReplicationLagEvaluator struct {
	tsPool     *pgxpool.Pool
	warningMB  float64
	criticalMB float64
}

func NewPgReplicationLagEvaluator(tsPool *pgxpool.Pool) *PgReplicationLagEvaluator {
	return &PgReplicationLagEvaluator{
		tsPool:     tsPool,
		warningMB:  100, // 100 MB
		criticalMB: 500, // 500 MB
	}
}

func (e *PgReplicationLagEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgReplicationLagEvaluator) Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT COALESCE(max_lag_mb, 0)
		FROM postgres_replication_stats
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1`

	var lagMB float64
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&lagMB); err != nil {
		if isNoDataError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("pg_replication_lag: %w", err)
	}

	var sev alerts.Severity
	if lagMB >= e.criticalMB {
		sev = alerts.SeverityCritical
	} else if lagMB >= e.warningMB {
		sev = alerts.SeverityWarning
	} else {
		return nil, nil
	}

	return []AlertEvaluatorResult{{
		RuleName:     "pg_replication_lag",
		Category:     "replication",
		Severity:     sev,
		Title:        fmt.Sprintf("PostgreSQL replication lag: %.1f MB", lagMB),
		Description:  fmt.Sprintf("Replication lag on %s is %.1f MB (threshold: %.0f MB warning, %.0f MB critical)", instanceName, lagMB, e.warningMB, e.criticalMB),
		InstanceName: instanceName,
		Engine:       alerts.EnginePostgres,
		Evidence: map[string]interface{}{
			"lag_mb":      lagMB,
			"warning_mb":  e.warningMB,
			"critical_mb": e.criticalMB,
		},
	}}, nil
}
