// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Alert evaluator – SQL Server disk space threshold detection.
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

// MssqlDiskSpaceEvaluator checks for low free disk space on SQL Server drives.
type MssqlDiskSpaceEvaluator struct {
	tsPool      *pgxpool.Pool
	warningPct  float64
	criticalPct float64
}

func NewMssqlDiskSpaceEvaluator(tsPool *pgxpool.Pool) *MssqlDiskSpaceEvaluator {
	return &MssqlDiskSpaceEvaluator{
		tsPool:      tsPool,
		warningPct:  20,
		criticalPct: 10,
	}
}

func (e *MssqlDiskSpaceEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *MssqlDiskSpaceEvaluator) Evaluate(ctx context.Context, instanceName string) ([]AlertEvaluatorResult, error) {
	const q = `
		SELECT database_name,
		       COALESCE(data_mb, 0) + COALESCE(log_mb, 0) AS used_mb,
		       COALESCE(free_mb, 0) AS free_mb
		FROM sqlserver_disk_history
		WHERE server_instance_name = $1
		  AND capture_timestamp >= now() - INTERVAL '15 minutes'
		ORDER BY capture_timestamp DESC
		LIMIT 20`

	rows, err := e.tsPool.Query(ctx, q, instanceName)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	var results []AlertEvaluatorResult
	seen := make(map[string]bool)

	for rows.Next() {
		var dbName string
		var usedMB, freeMB float64
		if err := rows.Scan(&dbName, &usedMB, &freeMB); err != nil {
			continue
		}
		if seen[dbName] {
			continue
		}
		seen[dbName] = true

		totalMB := usedMB + freeMB
		if totalMB <= 0 {
			continue
		}
		freePct := (freeMB / totalMB) * 100

		var sev alerts.Severity
		if freePct <= e.criticalPct {
			sev = alerts.SeverityCritical
		} else if freePct <= e.warningPct {
			sev = alerts.SeverityWarning
		} else {
			continue
		}

		results = append(results, AlertEvaluatorResult{
			RuleName:     "mssql_disk_space",
			Category:     "disk",
			Severity:     sev,
			Title:        fmt.Sprintf("SQL Server low disk: %s (%.1f%% free)", dbName, freePct),
			Description:  fmt.Sprintf("Database %s on %s has only %.1f%% free space (%.0f MB free of %.0f MB)", dbName, instanceName, freePct, freeMB, totalMB),
			InstanceName: instanceName,
			Engine:       alerts.EngineSQLServer,
			Evidence: map[string]interface{}{
				"database": dbName,
				"free_mb":  freeMB,
				"total_mb": totalMB,
				"free_pct": freePct,
			},
		})
	}
	return results, nil
}
