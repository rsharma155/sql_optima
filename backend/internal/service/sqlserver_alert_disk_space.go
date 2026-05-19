// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for SQL Server disk space usage.
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

type SqlServerDiskSpaceEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewSqlServerDiskSpaceEvaluator(tsPool *pgxpool.Pool) *SqlServerDiskSpaceEvaluator {
	return &SqlServerDiskSpaceEvaluator{tsPool: tsPool}
}

func (e *SqlServerDiskSpaceEvaluator) Engine() alerts.Engine { return alerts.EngineSQLServer }

func (e *SqlServerDiskSpaceEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT database_name, data_mb, log_mb, free_mb
		FROM sqlserver_disk_history
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '10 minutes'
		ORDER BY capture_timestamp DESC
	`
	rows, err := e.tsPool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlertEvaluatorResult
	serverName := serverID.String()

	seenDBs := make(map[string]bool)
	for rows.Next() {
		var dbName string
		var dataMB, logMB, freeMB float64
		if err := rows.Scan(&dbName, &dataMB, &logMB, &freeMB); err != nil {
			continue
		}
		if seenDBs[dbName] {
			continue
		}
		seenDBs[dbName] = true

		totalMB := dataMB + logMB + freeMB
		if totalMB > 0 {
			usedPct := ((dataMB + logMB) / totalMB) * 100.0
			if usedPct >= 90.0 {
				sev := alerts.SeverityWarning
				if usedPct >= 97.0 {
					sev = alerts.SeverityCritical
				}

				results = append(results, AlertEvaluatorResult{
					RuleName:    "MSDiskSpaceLow",
					Category:    "Storage",
					Severity:    sev,
					Title:       fmt.Sprintf("SQL Server disk space low on %s (%.1f%% used)", dbName, usedPct),
					Description: fmt.Sprintf("Database %s has %.1f%% disk usage. Free: %.1f MB", dbName, usedPct, freeMB),
					Evidence: map[string]interface{}{
						"database": dbName,
						"used_pct": usedPct,
						"free_mb":  freeMB,
					},
					ServerID:   serverID,
					ServerName: serverName,
					Engine:     alerts.EngineSQLServer,
				})
			}
		}
	}

	return results, nil
}
