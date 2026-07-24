// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for PostgreSQL connection pool / max_connections saturation.
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

type PgConnectionSaturationEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgConnectionSaturationEvaluator(tsPool *pgxpool.Pool) *PgConnectionSaturationEvaluator {
	return &PgConnectionSaturationEvaluator{tsPool: tsPool}
}

func (e *PgConnectionSaturationEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgConnectionSaturationEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT connections_max, connections_used, connections_usage_pct
		FROM postgres_control_center_stats
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var maxConn, used int
	var usagePct float64
	err := e.tsPool.QueryRow(ctx, q, serverID).Scan(&maxConn, &used, &usagePct)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if maxConn <= 0 || usagePct < 80.0 {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if usagePct >= 95.0 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName:    "PGConnectionSaturation",
		Category:    "Capacity",
		Severity:    sev,
		Title:       fmt.Sprintf("PostgreSQL connection usage high (%.1f%%)", usagePct),
		Description: fmt.Sprintf("Using %d of %d connections (%.1f%% of max_connections).", used, maxConn, usagePct),
		Evidence: map[string]interface{}{
			"connections_max":       maxConn,
			"connections_used":      used,
			"connections_usage_pct": usagePct,
		},
		ServerID:   serverID,
		ServerName: serverID.String(),
		Engine:     alerts.EnginePostgres,
	}}, nil
}
