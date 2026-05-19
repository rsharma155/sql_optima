// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for PostgreSQL disk space usage.
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

type PgDiskSpaceEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgDiskSpaceEvaluator(tsPool *pgxpool.Pool) *PgDiskSpaceEvaluator {
	return &PgDiskSpaceEvaluator{tsPool: tsPool}
}

func (e *PgDiskSpaceEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgDiskSpaceEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT mount_name, path, used_pct, total_bytes, free_bytes
		FROM postgres_disk_stats
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		ORDER BY capture_timestamp DESC
	`
	rows, err := e.tsPool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AlertEvaluatorResult
	serverName := serverID.String()

	seenMounts := make(map[string]bool)
	for rows.Next() {
		var mount, path string
		var usedPct float64
		var total, free int64
		if err := rows.Scan(&mount, &path, &usedPct, &total, &free); err != nil {
			continue
		}
		if seenMounts[mount] {
			continue
		}
		seenMounts[mount] = true

		if usedPct >= 85.0 {
			sev := alerts.SeverityWarning
			if usedPct >= 95.0 {
				sev = alerts.SeverityCritical
			}

			results = append(results, AlertEvaluatorResult{
				RuleName:    "PGDiskSpaceLow",
				Category:    "Storage",
				Severity:    sev,
				Title:       fmt.Sprintf("PostgreSQL disk space low on %s (%.1f%% used)", mount, usedPct),
				Description: fmt.Sprintf("Mount %s (%s) has %.1f%% disk usage. Free: %d MB", mount, path, usedPct, free/1024/1024),
				Evidence: map[string]interface{}{
					"mount":       mount,
					"path":        path,
					"used_pct":    usedPct,
					"free_bytes":  free,
					"total_bytes": total,
				},
				ServerID:   serverID,
				ServerName: serverName,
				Engine:     alerts.EnginePostgres,
			})
		}
	}

	return results, nil
}
