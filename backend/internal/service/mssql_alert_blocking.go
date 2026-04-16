// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert evaluator – SQL Server blocking session detection.
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

// MssqlBlockingEvaluator checks for active blocking chains in SQL Server.
type MssqlBlockingEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewMssqlBlockingEvaluator(tsPool *pgxpool.Pool) *MssqlBlockingEvaluator {
	return &MssqlBlockingEvaluator{tsPool: tsPool}
}

func (e *MssqlBlockingEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *MssqlBlockingEvaluator) Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT COALESCE(blocking_sessions, 0)
		FROM sqlserver_risk_health
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1`

	var blocking int
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&blocking); err != nil {
		return nil, nil // no data yet — not an error
	}
	if blocking == 0 {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if blocking >= 3 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName:     "mssql_blocking",
		Category:     "blocking",
		Severity:     sev,
		Title:        fmt.Sprintf("SQL Server blocking: %d sessions blocked", blocking),
		Description:  fmt.Sprintf("%d active blocking sessions detected on %s", blocking, instanceName),
		InstanceName: instanceName,
		Engine:       alerts.EngineSQLServer,
		Evidence:     map[string]interface{}{"blocking_sessions": blocking},
	}}, nil
}
