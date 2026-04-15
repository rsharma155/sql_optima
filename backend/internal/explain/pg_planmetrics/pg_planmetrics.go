// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Derive global metrics and time attribution from flattened PostgreSQL JSON plans.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_planmetrics

import (
	"math"
	"sort"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/pg_planparser"
)

type GlobalMetrics struct {
	TotalExecutionTimeMs float64 `json:"total_execution_time_ms"`
	PlanningTimeMs       float64 `json:"planning_time_ms,omitempty"`
	JitTimeMs            float64 `json:"jit_time_ms,omitempty"`
	MaxPlanDepth         int     `json:"max_plan_depth"`
	TotalNodes           int     `json:"total_nodes"`
	ParallelWorkersUsed  int     `json:"parallel_workers_used,omitempty"`
	MaxRowsProcessed     int64   `json:"max_rows_processed,omitempty"`
}

type TopNode struct {
	NodeID      int     `json:"node_id"`
	NodeType    string  `json:"node_type"`
	Relation    string  `json:"relation_name,omitempty"`
	TimeMs      float64 `json:"time_ms"`
	TimePercent float64 `json:"time_percent"`
	Depth       int     `json:"depth_level"`
}

type CategoryBreakdown struct {
	Category    string  `json:"category"`
	TimeMs      float64 `json:"time_ms"`
	TimePercent float64 `json:"time_percent"`
}

func ComputeGlobal(nodes []pg_planparser.FlatNode, planningTimeMs, totalExecutionTimeMs, jitTimeMs float64) GlobalMetrics {
	g := GlobalMetrics{
		TotalExecutionTimeMs: totalExecutionTimeMs,
		PlanningTimeMs:       planningTimeMs,
		JitTimeMs:            jitTimeMs,
		TotalNodes:           len(nodes),
	}
	for _, n := range nodes {
		if n.DepthLevel > g.MaxPlanDepth {
			g.MaxPlanDepth = n.DepthLevel
		}
		if n.WorkersLaunched > g.ParallelWorkersUsed {
			g.ParallelWorkersUsed = n.WorkersLaunched
		}
		if n.RowsProcessed > g.MaxRowsProcessed {
			g.MaxRowsProcessed = n.RowsProcessed
		}
	}
	return g
}

func NodeTimePercent(nodeExecMs, totalExecMs float64) float64 {
	if totalExecMs <= 0 || nodeExecMs <= 0 {
		return 0
	}
	return (nodeExecMs / totalExecMs) * 100.0
}

func TopTimeConsumingNodes(nodes []pg_planparser.FlatNode, totalExecMs float64, limit int) []TopNode {
	if limit <= 0 {
		limit = 10
	}
	out := make([]TopNode, 0, len(nodes))
	for _, n := range nodes {
		tp := NodeTimePercent(n.NodeExecutionTime, totalExecMs)
		out = append(out, TopNode{
			NodeID:      n.NodeID,
			NodeType:    n.NodeType,
			Relation:    n.RelationName,
			TimeMs:      n.NodeExecutionTime,
			TimePercent: tp,
			Depth:       n.DepthLevel,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TimeMs != out[j].TimeMs {
			return out[i].TimeMs > out[j].TimeMs
		}
		return out[i].NodeID < out[j].NodeID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// CategoryTimeAttribution groups node time into categories defined in enhance_analyze.md.
func CategoryTimeAttribution(nodes []pg_planparser.FlatNode, totalExecMs float64) []CategoryBreakdown {
	sum := map[string]float64{}
	for _, n := range nodes {
		cat := CategoryLabelForNodeType(n.NodeType)
		sum[cat] += n.NodeExecutionTime
	}
	out := make([]CategoryBreakdown, 0, len(sum))
	for cat, ms := range sum {
		out = append(out, CategoryBreakdown{
			Category:    cat,
			TimeMs:      ms,
			TimePercent: NodeTimePercent(ms, totalExecMs),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TimeMs != out[j].TimeMs {
			return out[i].TimeMs > out[j].TimeMs
		}
		return out[i].Category < out[j].Category
	})
	return out
}

// CategoryLabelForNodeType maps a plan node type to a coarse operation label (time breakdown buckets).
func CategoryLabelForNodeType(nodeType string) string {
	t := strings.ToLower(strings.TrimSpace(nodeType))
	switch {
	case strings.Contains(t, "sort"):
		return "Sort"
	case strings.Contains(t, "windowagg"):
		return "WindowAgg"
	case strings.Contains(t, "aggregate"):
		return "Aggregate"
	case strings.Contains(t, "hash join"):
		return "Hash Join"
	case strings.Contains(t, "merge join"):
		return "Merge Join"
	case strings.Contains(t, "nested loop"):
		return "Nested Loop"
	case strings.Contains(t, "gather merge"):
		return "Gather Merge"
	case strings.Contains(t, "gather"):
		return "Gather"
	case strings.Contains(t, "materialize"):
		return "Materialize"
	case strings.Contains(t, "cte scan"):
		return "CTE Scan"
	case strings.Contains(t, "subquery scan"):
		return "Subquery Scan"
	case strings.Contains(t, "bitmap heap scan"):
		return "Bitmap Heap Scan"
	case strings.Contains(t, "index scan") || strings.Contains(t, "index only scan"):
		return "Index Scan"
	case strings.Contains(t, "seq scan"):
		return "Seq Scan"
	case strings.Contains(t, "scan"):
		return "Scan"
	default:
		return "Other"
	}
}

// =============================================================================
// Cardinality estimation
// =============================================================================

type CardinalityNode struct {
	NodeID           int     `json:"node_id"`
	NodeType         string  `json:"node_type"`
	RelationName     string  `json:"relation_name,omitempty"`
	PlanRows         int     `json:"plan_rows,omitempty"`
	ActualRows       int     `json:"actual_rows,omitempty"`
	EstimationError  float64 `json:"estimation_error"`
	Classification   string  `json:"classification"`
}

type CardinalitySummary struct {
	WorstEstimationNode *CardinalityNode `json:"worst_estimation_node,omitempty"`
	AvgEstimationError  float64          `json:"avg_estimation_error,omitempty"`
}

func Cardinality(nodes []pg_planparser.FlatNode) CardinalitySummary {
	var (
		sumErr float64
		cnt    int
		worst  *CardinalityNode
	)
	for _, n := range nodes {
		if n.PlanRows <= 0 || n.ActualRows <= 0 {
			continue
		}
		err := float64(n.ActualRows) / float64(n.PlanRows)
		class := classifyEstimationError(err)
		node := &CardinalityNode{
			NodeID:          n.NodeID,
			NodeType:        n.NodeType,
			RelationName:    n.RelationName,
			PlanRows:        n.PlanRows,
			ActualRows:      n.ActualRows,
			EstimationError: err,
			Classification:  class,
		}
		sumErr += err
		cnt++
		if worst == nil {
			worst = node
			continue
		}
		if severityScore(node.EstimationError) > severityScore(worst.EstimationError) {
			worst = node
		}
	}
	var avg float64
	if cnt > 0 {
		avg = sumErr / float64(cnt)
	}
	return CardinalitySummary{WorstEstimationNode: worst, AvgEstimationError: avg}
}

func classifyEstimationError(r float64) string {
	if r <= 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		return "unknown"
	}
	if r > 10 || r < 0.1 {
		return "severe"
	}
	if r > 2 {
		return "underestimated"
	}
	if r < 0.5 {
		return "overestimated"
	}
	return "accurate"
}

// severityScore returns a symmetric “distance from 1.0” score for ranking worst estimation.
func severityScore(r float64) float64 {
	if r <= 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		return 0
	}
	if r >= 1 {
		return r
	}
	return 1.0 / r
}

