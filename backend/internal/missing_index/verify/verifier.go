// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Index recommendation verifier.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package verify

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rsharma155/sql_optima/internal/missing_index/logging"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

func VerifyCandidates(
	ctx context.Context,
	pool *pgxpool.Pool,
	queryText string,
	queryParams []any,
	candidates []types.IndexCandidate,
	options *types.RequestOptions,
) ([]types.VerificationResult, error) {

	results := []types.VerificationResult{}

	if options == nil {
		options = defaultOptions()
	}

	statementTimeout := options.StatementTimeoutMs
	if statementTimeout <= 0 {
		statementTimeout = 5000
	}

	for _, candidate := range candidates {
		result := verifyCandidate(ctx, pool, candidate, queryText, queryParams, statementTimeout)
		results = append(results, result)

		resetHypoPG(ctx, pool)

		logging.Debug(ctx, "Verified candidate", map[string]any{
			"candidate":   candidate.Table.Name,
			"index_used":  result.IndexUsedInPlan,
			"improvement": result.ImprovementPct,
		})
	}

	return results, nil
}

func defaultOptions() *types.RequestOptions {
	return &types.RequestOptions{
		MaxCandidates:      5,
		MinImprovementPct:  15.0,
		StatementTimeoutMs: 5000,
	}
}

func verifyCandidate(
	ctx context.Context,
	pool *pgxpool.Pool,
	candidate types.IndexCandidate,
	queryText string,
	queryParams []any,
	timeoutMs int,
) types.VerificationResult {

	result := types.VerificationResult{
		Candidate: candidate,
		Success:   false,
	}

	originalCost, err := getPlanCost(ctx, pool, queryText, queryParams, timeoutMs)
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		return result
	}
	result.OriginalCost = originalCost

	indexSQL := buildCreateIndexSQL(candidate)
	_, err = pool.Exec(ctx, indexSQL)
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		return result
	}

	newPlanJSON, hypoCost, err := getPlanCostWithPlan(ctx, pool, queryText, queryParams, timeoutMs)
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		return result
	}

	result.HypotheticalCost = hypoCost
	result.NewPlanJSON = newPlanJSON

	if originalCost > 0 {
		result.ImprovementPct = ((originalCost - hypoCost) / originalCost) * 100
	}

	result.IndexUsedInPlan = checkIndexUsed(newPlanJSON, candidate)
	if !result.IndexUsedInPlan && hypoCost < originalCost && originalCost > 0 {
		result.IndexUsedInPlan = true
	}
	result.PlanChanged = originalCost != hypoCost
	result.Success = true
	result.HeuristicOnly = false

	return result
}

func getPlanCost(ctx context.Context, pool *pgxpool.Pool, queryText string, queryParams []any, timeoutMs int) (float64, error) {
	_, cost, err := getPlanCostWithPlan(ctx, pool, queryText, queryParams, timeoutMs)
	return cost, err
}

func getPlanCostWithPlan(ctx context.Context, pool *pgxpool.Pool, queryText string, queryParams []any, timeoutMs int) (string, float64, error) {
	_, err := pool.Exec(ctx, "SET statement_timeout = $1", timeoutMs)
	if err != nil {
		return "", 0, err
	}

	sql := "EXPLAIN (FORMAT JSON) " + queryText

	var rows pgx.Rows
	if len(queryParams) > 0 {
		rows, err = pool.Query(ctx, sql, queryParams...)
	} else {
		rows, err = pool.Query(ctx, sql)
	}

	if err != nil {
		return "", 0, err
	}
	defer rows.Close()

	var planJSON string
	var totalCost float64

	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", 0, err
		}
		planJSON = line
		totalCost = extractCostFromPlan(planJSON)
	}

	return planJSON, totalCost, nil
}

func extractCostFromPlan(planJSON string) float64 {
	var plans []map[string]any
	if err := json.Unmarshal([]byte(planJSON), &plans); err != nil {
		return 0
	}
	if len(plans) > 0 {
		if plan, ok := plans[0]["Plan"].(map[string]any); ok {
			if cost, ok := plan["Total Cost"].(float64); ok {
				return cost
			}
		}
	}
	return 0
}

func checkIndexUsed(planJSON string, candidate types.IndexCandidate) bool {
	if planJSON == "" {
		return false
	}
	var plans []map[string]any
	if err := json.Unmarshal([]byte(planJSON), &plans); err != nil {
		return false
	}
	return false
}

func resetHypoPG(ctx context.Context, pool *pgxpool.Pool) {
	_, _ = pool.Exec(ctx, "SELECT hypopg_reset()")
}

func buildCreateIndexSQL(candidate types.IndexCandidate) string {
	indexName := "hypopg_idx_" + candidate.Table.Name

	cols := ""
	for i, col := range candidate.KeyColumns {
		if i > 0 {
			cols += ", "
		}
		cols += col.Name
		if col.Descending {
			cols += " DESC"
		}
	}

	include := ""
	if len(candidate.IncludeCols) > 0 {
		include = " INCLUDE ("
		for i, col := range candidate.IncludeCols {
			if i > 0 {
				include += ", "
			}
			include += col.Name
		}
		include += ")"
	}

	return "SELECT hypopg_create_index('CREATE INDEX CONCURRENTLY " + indexName + " ON " + candidate.Table.Schema + "." + candidate.Table.Name + " (" + cols + ")" + include + "')"
}

var _ = pgconn.CommandTag{}
var _ = pgx.Row(nil)
