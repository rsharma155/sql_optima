// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Cached EXPLAIN plan analysis handler (Timescale-backed plan_analysis_cache).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain"
	"github.com/rsharma155/sql_optima/internal/explain/analyzer"
	"github.com/rsharma155/sql_optima/internal/explain/pg_planparser"
	"github.com/rsharma155/sql_optima/internal/explain/pg_reportgenerator"
	"github.com/rsharma155/sql_optima/internal/repository"
	"github.com/rsharma155/sql_optima/internal/service"
)

// NewPgExplainAnalyzeHandler enhances PgExplainAnalyze with Timescale-backed caching when available.
// When Timescale isn't configured, it falls back to compute-on-demand behavior.
func NewPgExplainAnalyzeHandler(metricsSvc *service.MetricsService) http.HandlerFunc {
	var cacheRepo *repository.PlanAnalysisCacheRepository
	if metricsSvc != nil && metricsSvc.GetTimescaleDBPool() != nil {
		cacheRepo = repository.NewPlanAnalysisCacheRepository(metricsSvc.GetTimescaleDBPool())
	}

	// Use the existing global analyzers from pg_explain.go init().
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeExplainJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "error": "method not allowed"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxExplainPlanBodyBytes)

		var req pgExplainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": "invalid JSON body"})
			return
		}

		plan, raw, err := parseExplainPlanInputJSON(req.Plan)
		if err != nil {
			writeExplainJSON(w, http.StatusBadRequest, map[string]any{"success": false, "error": err.Error()})
			return
		}

		// Deterministic cache key based on canonical JSON.
		planHash := ""
		if cacheRepo != nil && cacheRepo.Enabled() {
			if h, err := pg_planparser.PlanHash([]byte(raw)); err == nil {
				planHash = h
			}
		}

		// Compute analyzer result (legacy findings / plan graph) always.
		result := pgExplainStdAnalyzer.Analyze(plan)
		flat := result.FlattenNodes()
		result.Plan.Plan = result.PlanTree
		queryText := strings.TrimSpace(req.Query)
		if queryText == "" {
			queryText = strings.TrimSpace(result.Plan.Query)
		}
		result.Query = queryText
		result.RawPlan = raw

		// Performance report: cacheable.
		var perfReport *pg_reportgenerator.Report
		fromCache := false
		if planHash != "" {
			if b, ok, err := cacheRepo.GetReportJSON(r.Context(), planHash); err == nil && ok && len(b) > 0 {
				var rep pg_reportgenerator.Report
				if err := json.Unmarshal(b, &rep); err == nil {
					perfReport = &rep
					fromCache = true
				}
			}
		}
		if perfReport == nil {
			perfReport = pg_reportgenerator.Generate(plan, pg_reportgenerator.DefaultOptions())
			if planHash != "" && cacheRepo != nil && cacheRepo.Enabled() {
				_ = cacheRepo.UpsertReport(r.Context(), planHash, []byte(raw), queryText, perfReport, plan.ExecutionTime)
			}
		}

		g := explain.BuildPlanGraph(result.Plan.Plan)
		bundle := explain.SQLContextBundle{Disclaimer: explain.SQLContextDisclaimer}
		if queryText != "" {
			bundle = explain.AugmentFindingsWithSQL(queryText, result.Findings, flat)
		}
		bundle.HeuristicInsights = explain.BuildHeuristicPlanInsights(&result.Plan.Plan, queryText)

		resp := map[string]any{
			"success":            true,
			"result":             result,
			"performance_report": perfReport,
			"performance_cached": fromCache,
			"plan_graph":         g,
			"plan_mermaid":       explain.MermaidFlowchart(g),
			"sql_context":        bundle,
		}
		writeExplainJSON(w, http.StatusOK, resp)
	}
}

// Ensure unused imports don't creep in when build tags change.
var _ = analyzer.New

