// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for SQL Server long-running queries from Timescale snapshots.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/domain/alerts"
)

const longRunningQueryThresholdMS = int64(5 * 60 * 1000) // 5 minutes

type SqlServerLongRunningQueryEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewSqlServerLongRunningQueryEvaluator(tsPool *pgxpool.Pool) *SqlServerLongRunningQueryEvaluator {
	return &SqlServerLongRunningQueryEvaluator{tsPool: tsPool}
}

func (e *SqlServerLongRunningQueryEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *SqlServerLongRunningQueryEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT COUNT(DISTINCT session_id)::int,
		       COALESCE(MAX(total_elapsed_time_ms), 0),
		       COALESCE(MAX(database_name), '')
		FROM sqlserver_long_running_queries
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		  AND total_elapsed_time_ms >= $2
	`
	var count int
	var maxElapsed int64
	var sampleDB string
	err := e.tsPool.QueryRow(ctx, q, serverID, longRunningQueryThresholdMS).Scan(&count, &maxElapsed, &sampleDB)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if maxElapsed >= 15*60*1000 || count >= 5 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName: "MSLongRunningQuery",
		Category: "Performance",
		Severity: sev,
		Title:    fmt.Sprintf("SQL Server long-running queries detected (%d)", count),
		Description: fmt.Sprintf(
			"%d session(s) running ≥5 minutes in the last sample window. Max elapsed %.1f min (sample DB: %s).",
			count, float64(maxElapsed)/60000.0, sampleDB,
		),
		Evidence: map[string]interface{}{
			"session_count":          count,
			"max_elapsed_time_ms":    maxElapsed,
			"threshold_ms":           longRunningQueryThresholdMS,
			"sample_database":        sampleDB,
		},
		ServerID:   serverID,
		ServerName: serverID.String(),
		Engine:     alerts.EngineSQLServer,
	}}, nil
}
