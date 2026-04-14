// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Feature extractor for index recommendation scoring.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package features

import (
	"context"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

type Features struct {
	NumTables       int  `json:"num_tables"`
	NumJoins        int  `json:"num_joins"`
	NumFilters      int  `json:"num_filters"`
	HasLimit        bool `json:"has_limit"`
	HasOffset       bool `json:"has_offset"`
	HasOrderBy      bool `json:"has_order_by"`
	HasGroupBy      bool `json:"has_group_by"`
	HasDistinct     bool `json:"has_distinct"`
	NumSubqueries   int  `json:"num_subqueries"`
	NumAggregates   int  `json:"num_aggregates"`
	HasSortPressure bool `json:"has_sort_pressure"`
}

type ExecutionFeedback struct {
	LatencyMs   float64 `json:"latency_ms"`
	RowsScanned int64   `json:"rows_scanned"`
	IndexUsed   string  `json:"index_used,omitempty"`
}

type CostModel interface {
	Predict(features Features) float64
}

type HeuristicModel struct{}

func NewHeuristicModel() *HeuristicModel {
	return &HeuristicModel{}
}

func (m *HeuristicModel) Predict(features Features) float64 {
	cost := 0.0

	cost += float64(features.NumTables) * 10
	cost += float64(features.NumJoins) * 20
	cost += float64(features.NumFilters) * 5

	if features.HasLimit {
		cost *= 0.5
	}

	if features.HasOffset {
		cost += 20
	}

	if features.HasSortPressure {
		cost += 30
	}

	if features.HasOrderBy {
		cost += 15
	}

	return cost
}

func ExtractFromQuery(ctx context.Context, analysis *types.QueryAnalysis) *Features {
	if analysis == nil {
		return &Features{}
	}

	features := &Features{
		NumTables:  len(analysis.Tables),
		NumJoins:   len(analysis.JoinInfo),
		NumFilters: len(analysis.Predicates),
		HasLimit:   analysis.Limit != nil,
		HasOrderBy: len(analysis.OrderBy) > 0,
	}

	return features
}

func ExtractFromPlan(ctx context.Context, planAnalysis *types.PlanAnalysis) *Features {
	if planAnalysis == nil {
		return &Features{}
	}

	features := &Features{}

	if planAnalysis.RootNode != nil {
		features.HasSortPressure = hasSortNode(planAnalysis.RootNode)
	}

	for _, opp := range planAnalysis.Opportunities {
		if opp.HasSortPressure {
			features.HasSortPressure = true
		}
	}

	return features
}

func hasSortNode(node *types.PlanNode) bool {
	if node == nil {
		return false
	}

	if node.NodeType == "Sort" || node.NodeType == "Incremental Sort" {
		return true
	}

	for _, child := range node.Children {
		if hasSortNode(child) {
			return true
		}
	}

	return false
}

func EstimateCost(model CostModel, features Features) float64 {
	if model == nil {
		defaultModel := NewHeuristicModel()
		return defaultModel.Predict(features)
	}
	return model.Predict(features)
}
