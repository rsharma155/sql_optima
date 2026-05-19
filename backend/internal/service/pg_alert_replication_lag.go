// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for PostgreSQL replication lag.
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

type PgReplicationLagEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgReplicationLagEvaluator(tsPool *pgxpool.Pool) *PgReplicationLagEvaluator {
	return &PgReplicationLagEvaluator{tsPool: tsPool}
}

func (e *PgReplicationLagEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgReplicationLagEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	q := `
		SELECT replica_name, lag_mb, state, sync_state
		FROM postgres_replication_lag_detail
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

	seenReplicas := make(map[string]bool)
	for rows.Next() {
		var name, state, syncState string
		var lagMB float64
		if err := rows.Scan(&name, &lagMB, &state, &syncState); err != nil {
			continue
		}
		if seenReplicas[name] {
			continue
		}
		seenReplicas[name] = true

		if lagMB >= 100.0 {
			sev := alerts.SeverityWarning
			if lagMB >= 1000.0 {
				sev = alerts.SeverityCritical
			}

			results = append(results, AlertEvaluatorResult{
				RuleName:    "PGReplicationLagHigh",
				Category:    "Replication",
				Severity:    sev,
				Title:       fmt.Sprintf("PostgreSQL replication lag high on %s (%.1f MB)", name, lagMB),
				Description: fmt.Sprintf("Replica %s is lagging by %.1f MB. State: %s, Sync: %s", name, lagMB, state, syncState),
				Evidence: map[string]interface{}{
					"replica_name": name,
					"lag_mb":       lagMB,
					"state":        state,
					"sync_state":   syncState,
				},
				ServerID:   serverID,
				ServerName: serverName,
				Engine:     alerts.EnginePostgres,
			})
		}
	}

	return results, nil
}
