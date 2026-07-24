// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for PgBouncer client wait pressure from postgres_pooler_stats.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

const (
	pgBouncerWaitClientsWarn = 5
	pgBouncerMaxWaitWarnSec  = 2.0
	pgBouncerWaitClientsCrit = 20
	pgBouncerMaxWaitCritSec  = 10.0
)

type PgBouncerWaitEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgBouncerWaitEvaluator(tsPool *pgxpool.Pool) *PgBouncerWaitEvaluator {
	return &PgBouncerWaitEvaluator{tsPool: tsPool}
}

func (e *PgBouncerWaitEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgBouncerWaitEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT COALESCE(cl_waiting, 0), COALESCE(maxwait_seconds, 0), COALESCE(cl_active, 0)
		FROM postgres_pooler_stats
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var waiting, active int
	var maxWait float64
	err := e.tsPool.QueryRow(ctx, q, serverID).Scan(&waiting, &maxWait, &active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if waiting < pgBouncerWaitClientsWarn && maxWait < pgBouncerMaxWaitWarnSec {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if waiting >= pgBouncerWaitClientsCrit || maxWait >= pgBouncerMaxWaitCritSec {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName: "PGBouncerWaitHigh",
		Category: "Capacity",
		Severity: sev,
		Title:    fmt.Sprintf("PgBouncer client wait high (%d waiting)", waiting),
		Description: fmt.Sprintf(
			"%d client(s) waiting (active=%d); maxwait %.1fs in the latest pooler sample.",
			waiting, active, maxWait,
		),
		Evidence: map[string]interface{}{
			"cl_waiting":      waiting,
			"cl_active":       active,
			"maxwait_seconds": maxWait,
		},
		ServerID:   serverID,
		ServerName: serverID.String(),
		Engine:     alerts.EnginePostgres,
	}}, nil
}
