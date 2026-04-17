// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Timescale-backed cache for deterministic EXPLAIN plan analysis reports.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const PlanAnalysisCacheSchemaVersion = 2

type PlanAnalysisCacheRow struct {
	PlanHash      string
	SchemaVersion int
	ReportJSON    []byte
}

type PlanAnalysisCacheRepository struct {
	pool *pgxpool.Pool
}

func NewPlanAnalysisCacheRepository(pool *pgxpool.Pool) *PlanAnalysisCacheRepository {
	return &PlanAnalysisCacheRepository{pool: pool}
}

func (r *PlanAnalysisCacheRepository) Enabled() bool {
	return r != nil && r.pool != nil
}

func (r *PlanAnalysisCacheRepository) GetReportJSON(ctx context.Context, planHash string) ([]byte, bool, error) {
	if !r.Enabled() {
		return nil, false, nil
	}
	var report []byte
	var version int
	err := r.pool.QueryRow(ctx, `
		SELECT report_json, schema_version
		FROM plan_analysis_cache
		WHERE plan_hash = $1
		LIMIT 1
	`, planHash).Scan(&report, &version)
	if err != nil {
		// pgx returns an error on no rows; treat as miss without importing pgx.ErrNoRows
		if stringsContains(err.Error(), "no rows") {
			return nil, false, nil
		}
		return nil, false, err
	}
	if version != PlanAnalysisCacheSchemaVersion {
		return nil, false, nil
	}
	return report, true, nil
}

func (r *PlanAnalysisCacheRepository) UpsertReport(ctx context.Context, planHash string, rawPlanJSON []byte, queryText string, report any, totalExecMs float64) error {
	if !r.Enabled() {
		return nil
	}
	b, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO plan_analysis_cache (plan_hash, schema_version, query_text, raw_plan_json, report_json, total_execution_time_ms)
		VALUES ($1, $2, NULLIF($3,''), $4::jsonb, $5::jsonb, $6)
		ON CONFLICT (plan_hash) DO UPDATE SET
			schema_version = EXCLUDED.schema_version,
			query_text = EXCLUDED.query_text,
			raw_plan_json = EXCLUDED.raw_plan_json,
			report_json = EXCLUDED.report_json,
			total_execution_time_ms = EXCLUDED.total_execution_time_ms,
			updated_at = NOW()
	`, planHash, PlanAnalysisCacheSchemaVersion, queryText, string(rawPlanJSON), string(b), totalExecMs)
	if err != nil {
		return err
	}
	return nil
}

func stringsContains(s, sub string) bool {
	if s == "" || sub == "" {
		return false
	}
	return strings.Contains(s, sub)
}
