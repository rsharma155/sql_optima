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
		SELECT count(DISTINCT p.blocked_pid) AS blocked_count
		FROM monitor.pg_blocking_pairs p
		JOIN optima_servers s ON p.server_id = s.id::text
		WHERE s.name = $1
		  AND p.collected_at >= now() - INTERVAL '5 minutes'`

	var blockedCount int
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&blockedCount); err != nil {
		return e.evaluateFromLockStats(ctx, instanceName)
	}

	var results []AlertEvaluatorResult

	if blockedCount > 0 {
		sev := alerts.SeverityWarning
		if blockedCount >= 3 {
			sev = alerts.SeverityCritical
		}
		results = append(results, AlertEvaluatorResult{
			RuleName:     "pg_blocking",
			Category:     "blocking",
			Severity:     sev,
			Title:        fmt.Sprintf("PostgreSQL blocking: %d sessions blocked", blockedCount),
			Description:  fmt.Sprintf("%d blocked sessions detected on %s", blockedCount, instanceName),
			InstanceName: instanceName,
			Engine:       alerts.EnginePostgres,
			Evidence:     map[string]interface{}{"blocked_sessions": blockedCount},
		})
	}

	// Duration threshold: alert if any session has been waiting > 60 s.
	const durQ = `
		SELECT COALESCE(MAX(waiting_seconds), 0)
		FROM monitor.pg_lock_snapshot l
		JOIN optima_servers s ON l.server_id = s.id::text
		WHERE s.name = $1
		  AND l.granted = false
		  AND l.collected_at >= now() - INTERVAL '2 minutes'`
	var maxWaitSec float64
	if err := e.tsPool.QueryRow(ctx, durQ, instanceName).Scan(&maxWaitSec); err == nil && maxWaitSec >= 60 {
		sev := alerts.SeverityWarning
		if maxWaitSec >= 180 {
			sev = alerts.SeverityCritical
		}
		results = append(results, AlertEvaluatorResult{
			RuleName:     "pg_lock_duration",
			Category:     "blocking",
			Severity:     sev,
			Title:        fmt.Sprintf("Lock wait duration: %.0f s on %s", maxWaitSec, instanceName),
			Description:  fmt.Sprintf("A session on %s has been waiting on a lock for %.0f seconds", instanceName, maxWaitSec),
			InstanceName: instanceName,
			Engine:       alerts.EnginePostgres,
			Evidence:     map[string]interface{}{"max_wait_seconds": maxWaitSec},
		})
	}

	// Idle-in-transaction: sessions idle in txn > 30 s while blocking is active.
	if blockedCount > 0 {
		const idleQ = `
			SELECT COUNT(DISTINCT s.pid)
			FROM monitor.pg_session_snapshot s
			JOIN optima_servers srv ON s.server_id = srv.id::text
			WHERE srv.name = $1
			  AND LOWER(COALESCE(s.state,'')) = 'idle in transaction'
			  AND s.collected_at >= now() - INTERVAL '2 minutes'
			  AND s.state_change IS NOT NULL
			  AND (s.collected_at - s.state_change) > INTERVAL '30 seconds'`
		var idleCount int
		if err := e.tsPool.QueryRow(ctx, idleQ, instanceName).Scan(&idleCount); err == nil && idleCount > 0 {
			results = append(results, AlertEvaluatorResult{
				RuleName:     "pg_idle_in_txn_blocking",
				Category:     "blocking",
				Severity:     alerts.SeverityCritical,
				Title:        fmt.Sprintf("Idle-in-transaction session blocking %d sessions on %s", blockedCount, instanceName),
				Description:  fmt.Sprintf("%d idle-in-transaction session(s) holding locks causing %d blocked sessions on %s", idleCount, blockedCount, instanceName),
				InstanceName: instanceName,
				Engine:       alerts.EnginePostgres,
				Evidence:     map[string]interface{}{"idle_in_txn_count": idleCount, "blocked_sessions": blockedCount},
			})
		}
	}

	if len(results) == 0 {
		return nil, nil
	}
	return results, nil
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
		if isNoDataError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("pg_blocking (lock_stats): %w", err)
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
