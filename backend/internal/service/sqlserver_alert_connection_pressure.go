// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for SQL Server user connection pressure from health KPI snapshots.
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

// Absolute fallbacks when max_connections is unset (0 = unlimited / unknown).
const (
	msUserConnWarn = 200
	msUserConnCrit = 500
	msConnPctWarn  = 80.0
	msConnPctCrit  = 95.0
)

type SqlServerConnectionPressureEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewSqlServerConnectionPressureEvaluator(tsPool *pgxpool.Pool) *SqlServerConnectionPressureEvaluator {
	return &SqlServerConnectionPressureEvaluator{tsPool: tsPool}
}

func (e *SqlServerConnectionPressureEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *SqlServerConnectionPressureEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT COALESCE(user_connections, 0), COALESCE(max_connections, 0)
		FROM sqlserver_health_kpis_v2
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		ORDER BY capture_timestamp DESC
		LIMIT 1
	`
	var userConn, maxConn int
	err := e.tsPool.QueryRow(ctx, q, serverID).Scan(&userConn, &maxConn)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var usagePct float64
	usePct := maxConn > 0
	if usePct {
		usagePct = (float64(userConn) / float64(maxConn)) * 100.0
		if usagePct < msConnPctWarn {
			return nil, nil
		}
	} else if userConn < msUserConnWarn {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if usePct {
		if usagePct >= msConnPctCrit {
			sev = alerts.SeverityCritical
		}
	} else if userConn >= msUserConnCrit {
		sev = alerts.SeverityCritical
	}

	title := fmt.Sprintf("SQL Server user connections high (%d)", userConn)
	desc := fmt.Sprintf("%d user process sessions (absolute warn≥%d).", userConn, msUserConnWarn)
	if usePct {
		title = fmt.Sprintf("SQL Server connection usage high (%.1f%%)", usagePct)
		desc = fmt.Sprintf("Using %d of %d user connections (%.1f%%).", userConn, maxConn, usagePct)
	}

	return []AlertEvaluatorResult{{
		RuleName:    "MSConnectionPressure",
		Category:    "Capacity",
		Severity:    sev,
		Title:       title,
		Description: desc,
		Evidence: map[string]interface{}{
			"user_connections": userConn,
			"max_connections":  maxConn,
			"usage_pct":        usagePct,
			"mode":             map[bool]string{true: "percent", false: "absolute"}[usePct],
		},
		ServerID:   serverID,
		ServerName: serverID.String(),
		Engine:     alerts.EngineSQLServer,
	}}, nil
}
