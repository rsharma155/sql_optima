// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Rule engine for EXPLAIN performance findings (disk spills, scans, joins, parallelism, etc.).
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_planrules

import (
	"math"
	"sort"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/pg_planmetrics"
	"github.com/rsharma155/sql_optima/internal/explain/pg_planparser"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type Finding struct {
	Code       string   `json:"code"`
	Severity   Severity `json:"severity"`
	Title      string   `json:"title"`
	Message    string   `json:"message"`
	NodeID     int      `json:"node_id,omitempty"`
	NodeType   string   `json:"node_type,omitempty"`
	Relation   string   `json:"relation_name,omitempty"`
	Evidence   map[string]any `json:"evidence,omitempty"`
	Suggestion string   `json:"suggestion,omitempty"`
}

type Options struct {
	LargeSeqScanRowThreshold int64
	ExcessiveNestedLoopRows  int64
	LargeIntermediateFactor  float64
}

func DefaultOptions() Options {
	return Options{
		LargeSeqScanRowThreshold: 100000,
		ExcessiveNestedLoopRows:  100000,
		LargeIntermediateFactor:  10.0,
	}
}

// DetectFindings implements enhance_analyze.md Step 6.
func DetectFindings(nodes []pg_planparser.FlatNode, totalExecMs float64, cardinality pg_planmetrics.CardinalitySummary, opts Options) []Finding {
	if opts.LargeSeqScanRowThreshold <= 0 {
		opts = DefaultOptions()
	}
	var out []Finding

	finalOutRows := int64(0)
	if len(nodes) > 0 {
		// Approximate final output rows as rows_processed at root.
		finalOutRows = nodes[0].RowsProcessed
	}
	if finalOutRows <= 0 {
		for _, n := range nodes {
			if n.RowsProcessed > finalOutRows {
				finalOutRows = n.RowsProcessed
			}
		}
	}
	if finalOutRows <= 0 {
		finalOutRows = 1
	}

	hasGather := false
	maxRows := int64(0)
	windowPct := 0.0
	for _, n := range nodes {
		maxRows = max64(maxRows, n.RowsProcessed)

		nt := strings.ToLower(n.NodeType)
		if strings.Contains(nt, "gather") {
			hasGather = true
		}
		if strings.Contains(nt, "windowagg") {
			windowPct += pg_planmetrics.NodeTimePercent(n.NodeExecutionTime, totalExecMs)
		}
		// Disk sort detection
		if strings.Contains(strings.ToLower(n.SortMethod), "external") || strings.EqualFold(n.SortSpaceType, "Disk") {
			out = append(out, Finding{
				Code:     "disk_sort",
				Severity: SeverityCritical,
				Title:    "Disk-based sort detected",
				Message:  "Query performs disk-based sort → work_mem insufficient or large global sort.",
				NodeID:   n.NodeID,
				NodeType: n.NodeType,
				Relation: n.RelationName,
				Evidence: map[string]any{
					"sort_method":     n.SortMethod,
					"sort_space_type": n.SortSpaceType,
					"sort_space_used": n.SortSpaceUsed,
				},
				Suggestion: "Increase work_mem for this session/query, reduce sort volume (filter earlier), or add an index supporting ORDER BY/GROUP BY.",
			})
		}
		// Hash spill detection
		if n.HashBatches > 1 {
			out = append(out, Finding{
				Code:     "hash_spill",
				Severity: SeverityCritical,
				Title:    "Hash join spilled to disk",
				Message:  "Hash join spilled to disk → insufficient work_mem.",
				NodeID:   n.NodeID,
				NodeType: n.NodeType,
				Relation: n.RelationName,
				Evidence: map[string]any{
					"hash_batches": n.HashBatches,
					"hash_buckets": n.HashBuckets,
				},
				Suggestion: "Increase work_mem, ensure join columns are indexed and statistics are current, or rewrite to reduce join input size.",
			})
		}
		// Sequential scan on large table
		if strings.Contains(nt, "seq scan") && n.RowsProcessed > opts.LargeSeqScanRowThreshold {
			out = append(out, Finding{
				Code:     "large_seq_scan",
				Severity: SeverityHigh,
				Title:    "Large sequential scan detected",
				Message:  "Sequential scan processes a large number of rows.",
				NodeID:   n.NodeID,
				NodeType: n.NodeType,
				Relation: n.RelationName,
				Evidence: map[string]any{"rows_processed": n.RowsProcessed},
				Suggestion: "Consider adding a selective index for filter/join columns, or reduce scanned rows via predicates/partitioning.",
			})
		}
		// Large intermediate result
		if float64(n.RowsProcessed) > opts.LargeIntermediateFactor*float64(finalOutRows) && n.RowsProcessed > 0 {
			out = append(out, Finding{
				Code:     "large_intermediate_result",
				Severity: SeverityHigh,
				Title:    "Large intermediate result",
				Message:  "Query generates a large intermediate dataset relative to final output.",
				NodeID:   n.NodeID,
				NodeType: n.NodeType,
				Relation: n.RelationName,
				Evidence: map[string]any{
					"rows_processed": n.RowsProcessed,
					"final_rows":     finalOutRows,
					"factor":         round2(float64(n.RowsProcessed) / float64(finalOutRows)),
				},
				Suggestion: "Filter earlier, reduce joins, or push down predicates to minimize intermediate row volume.",
			})
		}
		// Excessive nested loops
		if strings.Contains(nt, "nested loop") && n.RowsProcessed > opts.ExcessiveNestedLoopRows {
			out = append(out, Finding{
				Code:     "excessive_nested_loop",
				Severity: SeverityHigh,
				Title:    "Excessive nested loops",
				Message:  "Nested Loop processes high row volume; may indicate missing indexes or mis-estimates.",
				NodeID:   n.NodeID,
				NodeType: n.NodeType,
				Relation: n.RelationName,
				Evidence: map[string]any{"rows_processed": n.RowsProcessed},
				Suggestion: "Ensure join keys are indexed and statistics are up to date; consider rewriting to enable Hash/Merge join.",
			})
		}
	}

	// Expensive window function
	if windowPct > 20 {
		out = append(out, Finding{
			Code:       "expensive_windowagg",
			Severity:   SeverityHigh,
			Title:      "Expensive window function",
			Message:    "WindowAgg consumes a significant fraction of total runtime.",
			Evidence:   map[string]any{"windowagg_time_percent": round2(windowPct)},
			Suggestion: "Consider indexes supporting PARTITION BY/ORDER BY, reduce window frame size, or pre-aggregate where possible.",
		})
	}

	// Parallelism not used
	if !hasGather && maxRows >= 100000 {
		out = append(out, Finding{
			Code:       "parallelism_not_used",
			Severity:   SeverityMedium,
			Title:      "Parallelism not used",
			Message:    "No Gather/Gather Merge node found for a large row-volume plan.",
			Evidence:   map[string]any{"max_rows_processed": maxRows},
			Suggestion: "Check parallel settings (max_parallel_workers_per_gather, parallel_setup_cost, parallel_tuple_cost) and ensure the plan is parallel-safe.",
		})
	}

	// Cardinality note when severe
	if cardinality.WorstEstimationNode != nil && cardinality.WorstEstimationNode.Classification == "severe" {
		out = append(out, Finding{
			Code:     "severe_misestimation",
			Severity: SeverityHigh,
			Title:    "Severe row estimation error",
			Message:  "Planner row estimates differ drastically from actual rows; this can cascade into poor join choices and memory spills.",
			NodeID:   cardinality.WorstEstimationNode.NodeID,
			NodeType: cardinality.WorstEstimationNode.NodeType,
			Relation: cardinality.WorstEstimationNode.RelationName,
			Evidence: map[string]any{
				"estimated_rows": cardinality.WorstEstimationNode.PlanRows,
				"actual_rows":    cardinality.WorstEstimationNode.ActualRows,
				"error_ratio":    round2(cardinality.WorstEstimationNode.EstimationError),
			},
			Suggestion: "Run ANALYZE (or autoanalyze), check data skew, extended stats, and verify predicates are sargable.",
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		if sevRank(out[i].Severity) != sevRank(out[j].Severity) {
			return sevRank(out[i].Severity) > sevRank(out[j].Severity)
		}
		// prefer higher time percent when available
		ti := 0.0
		tj := 0.0
		if out[i].NodeID > 0 {
			ti = nodeTimePct(nodes, out[i].NodeID, totalExecMs)
		}
		if out[j].NodeID > 0 {
			tj = nodeTimePct(nodes, out[j].NodeID, totalExecMs)
		}
		if ti != tj {
			return ti > tj
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func nodeTimePct(nodes []pg_planparser.FlatNode, nodeID int, totalExecMs float64) float64 {
	for _, n := range nodes {
		if n.NodeID == nodeID {
			return pg_planmetrics.NodeTimePercent(n.NodeExecutionTime, totalExecMs)
		}
	}
	return 0
}

func sevRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 5
	case SeverityHigh:
		return 4
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func round2(x float64) float64 {
	if !isFinite(x) {
		return 0
	}
	return math.Round(x*100) / 100
}

func isFinite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

