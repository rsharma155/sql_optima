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
	rpoReplaySec := 60
	var policySec int
	if err := e.tsPool.QueryRow(ctx, `
		SELECT rpo_replay_seconds FROM optima_server_dr_policy WHERE server_id = $1`, serverID).Scan(&policySec); err == nil && policySec > 0 {
		rpoReplaySec = policySec
	}

	q := `
		SELECT DISTINCT ON (replica_name)
			replica_name, lag_mb, COALESCE(replay_lag_sec, 0), state, sync_state
		FROM postgres_replication_lag_detail
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		ORDER BY replica_name, capture_timestamp DESC
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
		var lagMB, replaySec float64
		if err := rows.Scan(&name, &lagMB, &replaySec, &state, &syncState); err != nil {
			continue
		}
		if seenReplicas[name] {
			continue
		}
		seenReplicas[name] = true

		lagByPolicy := replaySec > float64(rpoReplaySec)
		lagByMB := lagMB >= 100.0
		if !lagByPolicy && !lagByMB {
			continue
		}
		sev := alerts.SeverityWarning
		if replaySec > float64(rpoReplaySec*2) || lagMB >= 1000.0 {
			sev = alerts.SeverityCritical
		}
		title := fmt.Sprintf("PostgreSQL replication lag high on %s", name)
		desc := fmt.Sprintf("Replica %s: replay lag %.1fs (RPO %ds), %.1f MB. State: %s, Sync: %s",
			name, replaySec, rpoReplaySec, lagMB, state, syncState)
		if lagByPolicy {
			title = fmt.Sprintf("PostgreSQL replay lag exceeds RPO on %s (%.0fs)", name, replaySec)
		} else if lagByMB {
			title = fmt.Sprintf("PostgreSQL replication lag high on %s (%.1f MB)", name, lagMB)
			desc = fmt.Sprintf("Replica %s is lagging by %.1f MB. State: %s, Sync: %s", name, lagMB, state, syncState)
		}

		results = append(results, AlertEvaluatorResult{
			RuleName:    "PGReplicationLagHigh",
			Category:    "Replication",
			Severity:    sev,
			Title:       title,
			Description: desc,
			Evidence: map[string]interface{}{
				"replica_name":      name,
				"lag_mb":            lagMB,
				"replay_lag_sec":    replaySec,
				"rpo_replay_seconds": rpoReplaySec,
				"state":             state,
				"sync_state":        syncState,
			},
			ServerID:   serverID,
			ServerName: serverName,
			Engine:     alerts.EnginePostgres,
		})
	}

	return results, nil
}
