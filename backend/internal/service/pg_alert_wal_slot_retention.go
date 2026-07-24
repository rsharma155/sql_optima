// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Evaluator for PostgreSQL replication-slot WAL retention growth.
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

type PgWalSlotRetentionEvaluator struct {
	tsPool *pgxpool.Pool
}

func NewPgWalSlotRetentionEvaluator(tsPool *pgxpool.Pool) *PgWalSlotRetentionEvaluator {
	return &PgWalSlotRetentionEvaluator{tsPool: tsPool}
}

func (e *PgWalSlotRetentionEvaluator) Engine() alerts.Engine { return alerts.EnginePostgres }

func (e *PgWalSlotRetentionEvaluator) Evaluate(ctx context.Context, serverID uuid.UUID) ([]AlertEvaluatorResult, error) {
	maxGB := 10.0
	var policyGB float64
	if err := e.tsPool.QueryRow(ctx, `
		SELECT max_slot_retention_gb FROM optima_server_dr_policy WHERE server_id = $1`, serverID).Scan(&policyGB); err == nil && policyGB > 0 {
		maxGB = policyGB
	}
	maxMB := maxGB * 1024.0

	q := `
		SELECT DISTINCT ON (slot_name)
			slot_name, COALESCE(slot_type, ''), active, COALESCE(retained_wal_mb, 0)
		FROM postgres_replication_slot_stats
		WHERE server_id = $1
		  AND capture_timestamp >= now() - interval '5 minutes'
		ORDER BY slot_name, capture_timestamp DESC
	`
	rows, err := e.tsPool.Query(ctx, q, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var worstName, worstType string
	var worstActive bool
	var worstMB float64
	var overCount int

	for rows.Next() {
		var name, slotType string
		var active bool
		var retainedMB float64
		if err := rows.Scan(&name, &slotType, &active, &retainedMB); err != nil {
			continue
		}
		if retainedMB < maxMB {
			continue
		}
		overCount++
		if retainedMB > worstMB {
			worstMB = retainedMB
			worstName = name
			worstType = slotType
			worstActive = active
		}
	}
	if overCount == 0 {
		return nil, nil
	}

	sev := alerts.SeverityWarning
	if worstMB >= maxMB*2 {
		sev = alerts.SeverityCritical
	}

	return []AlertEvaluatorResult{{
		RuleName: "PGWalSlotRetentionHigh",
		Category: "Replication",
		Severity: sev,
		Title:    fmt.Sprintf("PostgreSQL WAL slot retention high on %s (%.1f MB)", worstName, worstMB),
		Description: fmt.Sprintf(
			"Slot %s retains %.1f MB of WAL (policy max %.1f GB). %d slot(s) over threshold. Active=%v type=%s.",
			worstName, worstMB, maxGB, overCount, worstActive, worstType,
		),
		Evidence: map[string]interface{}{
			"slot_name":             worstName,
			"slot_type":             worstType,
			"active":                worstActive,
			"retained_wal_mb":       worstMB,
			"max_slot_retention_gb": maxGB,
			"slots_over_threshold":  overCount,
		},
		ServerID:   serverID,
		ServerName: serverID.String(),
		Engine:     alerts.EnginePostgres,
	}}, nil
}
