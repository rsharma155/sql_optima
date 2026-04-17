// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert evaluator – PostgreSQL blocking chain detection.
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

// PgBlockingEvaluator checks for active blocking chains in PostgreSQL.
type PgBlockingEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgBlockingEvaluator(tsPool *pgxpool.Pool) *PgBlockingEvaluator {
	return &PgBlockingEvaluator{tsPool: tsPool}
}

func (e *PgBlockingEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgBlockingEvaluator) Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT count(DISTINCT blocked_pid) AS blocked_count
		FROM monitor.pg_blocking_pairs
		WHERE server_instance_name = $1
		  AND collected_at >= now() - INTERVAL '5 minutes'`

	var blockedCount int
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&blockedCount); err != nil {
		// Fallback: try postgres_lock_stats
		return e.evaluateFromLockStats(ctx, instanceName)
	}
	if blockedCount == 0 {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if blockedCount >= 3 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName:     "pg_blocking",
		Category:     "blocking",
		Severity:     sev,
		Title:        fmt.Sprintf("PostgreSQL blocking: %d sessions blocked", blockedCount),
		Description:  fmt.Sprintf("%d blocked sessions detected on %s", blockedCount, instanceName),
		InstanceName: instanceName,
		Engine:       alerts.EnginePostgres,
		Evidence:     map[string]interface{}{"blocked_sessions": blockedCount},
	}}, nil
}

func (e *PgBlockingEvaluator) evaluateFromLockStats(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT count(*) AS waiting_count
		FROM postgres_lock_stats
		WHERE server_instance_name = $1
		  AND granted = false
		  AND capture_timestamp >= now() - INTERVAL '5 minutes'`

	var waiting int
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&waiting); err != nil {
		return nil, nil
	}
	if waiting == 0 {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if waiting >= 5 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName:     "pg_blocking",
		Category:     "blocking",
		Severity:     sev,
		Title:        fmt.Sprintf("PostgreSQL: %d sessions waiting for locks", waiting),
		Description:  fmt.Sprintf("%d sessions waiting for locks on %s", waiting, instanceName),
		InstanceName: instanceName,
		Engine:       alerts.EnginePostgres,
		Evidence:     map[string]interface{}{"waiting_sessions": waiting},
	}}, nil
}
