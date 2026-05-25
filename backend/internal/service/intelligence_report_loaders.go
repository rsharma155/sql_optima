// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: P1 telemetry loaders for the SQL Server Intelligence Report.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rsharma155/sql_optima/internal/models"
)

func (s *IntelligenceReportService) loadQueryWorkloadSummary(ctx context.Context, serverID uuid.UUID) (*models.QueryWorkloadSummary, error) {
	since := time.Now().Add(-24 * time.Hour)
	summary := &models.QueryWorkloadSummary{}

	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sqlserver_query_regressions
		WHERE server_id = $1 AND capture_timestamp >= $2
	`, serverID, since).Scan(&summary.Regressions24h)

	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT query_hash) FROM sqlserver_plan_instability
		WHERE server_id = $1 AND capture_timestamp >= $2
	`, serverID, since).Scan(&summary.PlanInstabilityQueries)

	var qh int64
	var qtext string
	var cpuMs float64
	err := s.pool.QueryRow(ctx, `
		SELECT query_hash, LEFT(COALESCE(statement_text, query_text_raw, ''), 200), COALESCE(total_cpu_ms, 0)
		FROM sqlserver_query_metrics_v2
		WHERE server_id = $1
		  AND capture_timestamp >= $2
		  AND COALESCE(total_cpu_ms, 0) > 0
		ORDER BY total_cpu_ms DESC NULLS LAST
		LIMIT 1
	`, serverID, since).Scan(&qh, &qtext, &cpuMs)
	if err == nil && qh != 0 {
		summary.TopCPUQueryHash = fmt.Sprintf("0x%X", uint64(qh))
		summary.TopCPUQueryText = strings.TrimSpace(qtext)
		summary.TopCPUMs = cpuMs
	}
	return summary, nil
}

func (s *IntelligenceReportService) loadPerformanceDebtFindings(ctx context.Context, serverID uuid.UUID, limit int) ([]models.PerformanceDebtFindingSummary, error) {
	if limit <= 0 {
		limit = 15
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (finding_key)
			section, finding_type, severity, title, database_name, object_name,
			COALESCE(recommendation, '')
		FROM sqlserver_performance_debt_findings
		WHERE server_id = $1
		  AND capture_timestamp >= NOW() - INTERVAL '7 days'
		  AND severity IN ('CRITICAL', 'HIGH', 'WARNING')
		ORDER BY finding_key, capture_timestamp DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.PerformanceDebtFindingSummary
	for rows.Next() {
		var f models.PerformanceDebtFindingSummary
		if scanErr := rows.Scan(&f.Section, &f.FindingType, &f.Severity, &f.Title,
			&f.DatabaseName, &f.ObjectName, &f.Recommendation); scanErr == nil {
			out = append(out, f)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, rows.Err()
}

func (s *IntelligenceReportService) loadIndexHealthTopN(ctx context.Context, serverID uuid.UUID, limit int) ([]models.IndexHealthEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	var out []models.IndexHealthEntry

	fragRows2, err2 := s.pool.Query(ctx, `
		SELECT database_name, schema_name, table_name, index_name, avg_fragmentation_pct, page_count
		FROM sqlserver_index_fragmentation
		WHERE server_id = $1
		  AND capture_timestamp = (
			SELECT MAX(capture_timestamp) FROM sqlserver_index_fragmentation WHERE server_id = $1
		  )
		  AND avg_fragmentation_pct >= 30
		  AND page_count >= 1000
		ORDER BY avg_fragmentation_pct DESC
		LIMIT $2
	`, serverID, limit)
	if err2 == nil {
		defer fragRows2.Close()
		for fragRows2.Next() {
			var e models.IndexHealthEntry
			var fragPct float64
			var pageCount int64
			if scanErr := fragRows2.Scan(&e.DatabaseName, &e.SchemaName, &e.TableName, &e.IndexName,
				&fragPct, &pageCount); scanErr == nil {
				e.FindingKind = "fragmentation"
				e.MetricLabel = "avg_fragmentation_pct"
				e.MetricValue = fragPct
				if fragPct >= 50 {
					e.Severity = "high"
				} else {
					e.Severity = "medium"
				}
				e.Recommendation = fmt.Sprintf("Rebuild or reorganize (%d pages)", pageCount)
				out = append(out, e)
			}
		}
	}

	missRows, err3 := s.pool.Query(ctx, `
		SELECT database_name, schema_name, table_name, improvement_score, equality_columns
		FROM sqlserver_missing_indexes
		WHERE server_id = $1
		  AND capture_timestamp = (
			SELECT MAX(capture_timestamp) FROM sqlserver_missing_indexes WHERE server_id = $1
		  )
		ORDER BY improvement_score DESC
		LIMIT $2
	`, serverID, limit)
	if err3 == nil {
		defer missRows.Close()
		for missRows.Next() {
			var e models.IndexHealthEntry
			var eqCols string
			if scanErr := missRows.Scan(&e.DatabaseName, &e.SchemaName, &e.TableName,
				&e.MetricValue, &eqCols); scanErr == nil {
				e.FindingKind = "missing_index"
				e.MetricLabel = "improvement_score"
				e.Severity = "medium"
				e.Recommendation = "Consider index on: " + truncateStr(eqCols, 120)
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func truncateStr(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (s *IntelligenceReportService) loadServerConfigurationSnapshot(ctx context.Context, serverID uuid.UUID) (*models.ServerConfigurationSnapshot, map[string]interface{}) {
	raw := make(map[string]interface{})
	cfg := &models.ServerConfigurationSnapshot{}

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (object_name)
			object_name, details
		FROM sqlserver_performance_debt_findings
		WHERE server_id = $1
		  AND section = 'Engine Config'
		  AND capture_timestamp >= NOW() - INTERVAL '7 days'
		ORDER BY object_name, capture_timestamp DESC
	`, serverID)
	if err != nil {
		return nil, raw
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var details []byte
		if scanErr := rows.Scan(&name, &details); scanErr != nil {
			continue
		}
		var m map[string]interface{}
		if len(details) > 0 && json.Unmarshal(details, &m) == nil {
			if v, ok := toFloatFromJSON(m["value_in_use"]); ok {
				switch name {
				case "max degree of parallelism":
					cfg.MaxDegreeOfParallelism = int(v)
					raw["max_degree_of_parallelism"] = v
				case "max server memory (MB)":
					cfg.MaxServerMemoryMB = int(v)
					raw["max_server_memory_mb"] = v
				case "cost threshold for parallelism":
					cfg.CostThresholdForParallelism = int(v)
					raw["cost_threshold_for_parallelism"] = v
				}
			}
		}
	}
	if cfg.MaxDegreeOfParallelism == 0 && cfg.MaxServerMemoryMB == 0 && cfg.CostThresholdForParallelism == 0 {
		return nil, raw
	}
	return cfg, raw
}

func toFloatFromJSON(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func attachQueryWorkloadRules(result *models.IntelligenceReportResponse, qw *models.QueryWorkloadSummary) {
	if qw == nil {
		return
	}
	if qw.Regressions24h > 0 {
		sev := "medium"
		if qw.Regressions24h >= 5 {
			sev = "high"
		}
		r := models.RuleTriggerResult{
			RuleName: "query_store_regressions",
			Severity: sev,
			Message:  fmt.Sprintf("%d Query Store regression(s) detected in the last 24 hours.", qw.Regressions24h),
			MetricValues: map[string]float64{
				"query_regressions_24h": float64(qw.Regressions24h),
			},
			Recommendation: []string{
				"Open Query Analysis and review regressions by percent change",
				"Compare execution plans for regressed query hashes",
			},
		}
		result.TriggeredRules = append(result.TriggeredRules, r)
		result.CurrentIssues = append(result.CurrentIssues, ruleToIssueMap(r))
	}
	if qw.PlanInstabilityQueries >= 3 {
		r := models.RuleTriggerResult{
			RuleName: "query_plan_instability",
			Severity: "medium",
			Message:  fmt.Sprintf("%d queries with multiple active plans in the last 24 hours.", qw.PlanInstabilityQueries),
			MetricValues: map[string]float64{
				"plan_instability_queries": float64(qw.PlanInstabilityQueries),
			},
			Recommendation: []string{
				"Review plan instability in Query Analysis",
				"Check for parameter sniffing or recent statistics updates",
			},
		}
		result.TriggeredRules = append(result.TriggeredRules, r)
		result.CurrentIssues = append(result.CurrentIssues, ruleToIssueMap(r))
	}
}

func ruleToIssueMap(r models.RuleTriggerResult) map[string]interface{} {
	rec := ""
	if len(r.Recommendation) > 0 {
		rec = r.Recommendation[0]
	}
	return map[string]interface{}{
		"title":          r.RuleName,
		"description":    r.Message,
		"severity":       r.Severity,
		"recommendation": rec,
		"metric_values":  r.MetricValues,
	}
}

func mergePerformanceDebtIntoIssues(result *models.IntelligenceReportResponse, findings []models.PerformanceDebtFindingSummary) {
	existing := make(map[string]bool, len(result.CurrentIssues))
	for _, iss := range result.CurrentIssues {
		if t, ok := iss["title"].(string); ok {
			existing[t] = true
		}
	}
	for _, f := range findings {
		if existing[f.Title] {
			continue
		}
		sev := strings.ToLower(f.Severity)
		if sev == "warning" {
			sev = "medium"
		}
		result.CurrentIssues = append(result.CurrentIssues, map[string]interface{}{
			"title":          f.Title,
			"description":    fmt.Sprintf("[%s] %s — %s", f.Section, f.FindingType, f.ObjectName),
			"severity":       sev,
			"recommendation": f.Recommendation,
			"source":         "performance_debt",
		})
		existing[f.Title] = true
	}
}
