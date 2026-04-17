// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert evaluator – PostgreSQL disk space threshold detection.
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

// PgDiskSpaceEvaluator checks for low free disk space on PostgreSQL hosts.
type PgDiskSpaceEvaluator struct {
	tsPool      *pgxpool.Pool
	warningPct  float64
	criticalPct float64
}

func NewPgDiskSpaceEvaluator(tsPool *pgxpool.Pool) *PgDiskSpaceEvaluator {
	return &PgDiskSpaceEvaluator{
		tsPool:      tsPool,
		warningPct:  20,
		criticalPct: 10,
	}
}

func (e *PgDiskSpaceEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgDiskSpaceEvaluator) Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT COALESCE(disk_total_gb, 0), COALESCE(disk_used_gb, 0)
		FROM system_stats_detail
		WHERE server_instance_name = $1
		ORDER BY capture_timestamp DESC
		LIMIT 1`

	var totalGB, usedGB float64
	if err := e.tsPool.QueryRow(ctx, q, instanceName).Scan(&totalGB, &usedGB); err != nil {
		if isNoDataError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("pg_disk_space: %w", err)
	}
	if totalGB <= 0 {
		return nil, nil
	}

	freeGB := totalGB - usedGB
	freePct := (freeGB / totalGB) * 100

	var sev alerts.Severity
	if freePct <= e.criticalPct {
		sev = alerts.SeverityCritical
	} else if freePct <= e.warningPct {
		sev = alerts.SeverityWarning
	} else {
		return nil, nil
	}

	return []AlertEvaluatorResult{{
		RuleName:     "pg_disk_space",
		Category:     "disk",
		Severity:     sev,
		Title:        fmt.Sprintf("PostgreSQL low disk: %.1f%% free (%.1f GB)", freePct, freeGB),
		Description:  fmt.Sprintf("Host for %s has only %.1f%% disk space free (%.1f GB of %.1f GB)", instanceName, freePct, freeGB, totalGB),
		InstanceName: instanceName,
		Engine:       alerts.EnginePostgres,
		Evidence: map[string]interface{}{
			"free_gb":  freeGB,
			"total_gb": totalGB,
			"free_pct": freePct,
		},
	}}, nil
}
