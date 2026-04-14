// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Provides joinoptimizer functionality for the backend.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package joinoptimizer

import (
	"math"
	"sort"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

type State struct {
	Tables       []types.TableRef
	JoinGraph    types.JoinGraph
	Filters      []types.Predicate
	AvailableIdx map[string][]types.ExistingIndex
}

type Action struct {
	JoinOrder  []types.TableRef
	JoinMethod string
}

type Policy struct {
	selectivityCache map[string]float64
}

func NewPolicy() *Policy {
	return &Policy{
		selectivityCache: make(map[string]float64),
	}
}

func (p *Policy) SelectJoinOrder(state *State) *types.JoinOrderResult {
	if len(state.Tables) == 0 {
		return &types.JoinOrderResult{
			JoinOrder:     []string{},
			EstimatedCost: 0,
			PolicyUsed:    "empty",
		}
	}

	if len(state.Tables) == 1 {
		return &types.JoinOrderResult{
			JoinOrder:     []string{state.Tables[0].Name},
			JoinMethods:   []string{"seq_scan"},
			EstimatedCost: 100.0,
			PolicyUsed:    "single_table",
		}
	}

	edges := state.JoinGraph.Edges

	joinOrder := p.computeOptimalJoinOrder(state.Tables, edges, state.Filters)

	joinMethods := make([]string, len(joinOrder))
	for i := range joinOrder {
		joinMethods[i] = p.selectJoinMethod(joinOrder[i], state)
	}

	estimatedCost := p.estimateJoinOrderCost(joinOrder, state)
	rowPenalty := p.calculateRowExplosionPenalty(joinOrder, state)

	return &types.JoinOrderResult{
		JoinOrder:           joinOrder,
		JoinMethods:         joinMethods,
		EstimatedCost:       estimatedCost,
		RowExplosionPenalty: rowPenalty,
		Confidence:          p.calculateConfidence(joinOrder, state),
		PolicyUsed:          "rl_heuristic",
	}
}

func (p *Policy) computeOptimalJoinOrder(tables []types.TableRef, edges []types.JoinEdge, filters []types.Predicate) []string {
	if len(tables) <= 2 {
		order := make([]string, len(tables))
		for i, t := range tables {
			order[i] = t.Name
		}
		return order
	}

	selectivities := make(map[string]float64)
	for _, edge := range edges {
		key := edge.LeftTable.Name + "-" + edge.RightTable.Name
		selectivities[key] = edge.Selectivity
		if selectivities[key] == 0 {
			selectivities[key] = p.estimateJoinSelectivity(edge, filters)
		}
	}

	for _, f := range filters {
		for i := range tables {
			if tables[i].Name == f.Table.Name {
				sel := p.estimateFilterSelectivity(f)
				key := tables[i].Name + "_filter"
				selectivities[key] = sel
			}
		}
	}

	var ordered []string
	remaining := make([]string, len(tables))
	for i, t := range tables {
		remaining[i] = t.Name
	}

	for len(remaining) > 0 {
		bestIdx := 0
		bestScore := -math.MaxFloat64

		for i, tableName := range remaining {
			score := p.calculateJoinPriorityScore(tableName, remaining, selectivities, edges)
			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}

		ordered = append(ordered, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return ordered
}

func (p *Policy) calculateJoinPriorityScore(tableName string, remaining []string, selectivities map[string]float64, edges []types.JoinEdge) float64 {
	score := 0.0

	connectedCount := 0
	for _, edge := range edges {
		if edge.LeftTable.Name == tableName || edge.RightTable.Name == tableName {
			connectedCount++
			for _, other := range remaining {
				if other == edge.LeftTable.Name || other == edge.RightTable.Name {
					if other != tableName {
						key := tableName + "-" + other
						if sel, ok := selectivities[key]; ok {
							score += (1.0 - sel) * 10
						}
					}
				}
			}
		}
	}

	score += float64(connectedCount) * 5

	key := tableName + "_filter"
	if sel, ok := selectivities[key]; ok {
		score += (1.0 - sel) * 20
	}

	return score
}

func (p *Policy) estimateJoinSelectivity(edge types.JoinEdge, filters []types.Predicate) float64 {
	key := edge.LeftTable.Name + "-" + edge.RightTable.Name
	if sel, ok := p.selectivityCache[key]; ok {
		return sel
	}

	sel := 0.1
	for _, f := range filters {
		if f.Table.Name == edge.LeftTable.Name || f.Table.Name == edge.RightTable.Name {
			fsel := p.estimateFilterSelectivity(f)
			sel *= fsel
		}
	}

	sel = math.Max(0.001, math.Min(0.5, sel))
	p.selectivityCache[key] = sel
	return sel
}

func (p *Policy) estimateFilterSelectivity(pred types.Predicate) float64 {
	switch pred.Type {
	case types.PredicateTypeEquality:
		return 0.01
	case types.PredicateTypeRange:
		return 0.1
	case types.PredicateTypeIn:
		return 0.05
	case types.PredicateTypeIsNull:
		return 0.05
	default:
		return 0.1
	}
}

func (p *Policy) selectJoinMethod(table string, state *State) string {
	indexes, ok := state.AvailableIdx[table]
	if ok && len(indexes) > 0 {
		return "index_scan"
	}
	return "seq_scan"
}

func (p *Policy) estimateJoinOrderCost(joinOrder []string, state *State) float64 {
	cost := 0.0

	for i, table := range joinOrder {
		tableCost := 100.0

		idx, ok := state.AvailableIdx[table]
		if ok && len(idx) > 0 {
			tableCost = 10.0
		}

		for _, f := range state.Filters {
			if f.Table.Name == table {
				switch f.Type {
				case types.PredicateTypeEquality:
					tableCost *= 0.5
				case types.PredicateTypeRange:
					tableCost *= 0.7
				}
			}
		}

		if i > 0 {
			cost += tableCost * float64(i+1)
		} else {
			cost += tableCost
		}
	}

	return cost
}

func (p *Policy) calculateRowExplosionPenalty(joinOrder []string, state *State) float64 {
	penalty := 0.0

	edgeMap := make(map[string]types.JoinEdge)
	for _, edge := range state.JoinGraph.Edges {
		key := edge.LeftTable.Name + "-" + edge.RightTable.Name
		edgeMap[key] = edge
	}

	rows := 1.0
	for i := 0; i < len(joinOrder)-1; i++ {
		key1 := joinOrder[i] + "-" + joinOrder[i+1]
		key2 := joinOrder[i+1] + "-" + joinOrder[i]

		sel := 0.1
		if edge, ok := edgeMap[key1]; ok {
			sel = edge.Selectivity
		} else if edge, ok := edgeMap[key2]; ok {
			sel = edge.Selectivity
		}

		rows = rows * sel * 1000
		if rows > 1000000 {
			penalty += (rows - 1000000) / 100000
		}
	}

	return math.Min(1.0, penalty/10)
}

func (p *Policy) calculateConfidence(joinOrder []string, state *State) float64 {
	confidence := 0.5

	if len(state.Tables) <= 3 {
		confidence += 0.2
	}

	if len(state.JoinGraph.Edges) > 0 {
		confidence += 0.15
	}

	hasFilters := false
	for _, f := range state.Filters {
		if f.Type == types.PredicateTypeEquality {
			hasFilters = true
			break
		}
	}
	if hasFilters {
		confidence += 0.15
	}

	return math.Min(1.0, confidence)
}

type JoinOrderOptimizer struct {
	policy *Policy
}

func New() *JoinOrderOptimizer {
	return &JoinOrderOptimizer{
		policy: NewPolicy(),
	}
}

func (o *JoinOrderOptimizer) Optimize(queryAnalysis *types.QueryAnalysis, tableInfos map[string][]types.ExistingIndex) *types.JoinOrderResult {
	state := &State{
		Tables:       queryAnalysis.Tables,
		JoinGraph:    buildJoinGraph(queryAnalysis),
		Filters:      queryAnalysis.Predicates,
		AvailableIdx: tableInfos,
	}

	return o.policy.SelectJoinOrder(state)
}

func buildJoinGraph(qa *types.QueryAnalysis) types.JoinGraph {
	graph := types.JoinGraph{
		Tables:  qa.Tables,
		Edges:   []types.JoinEdge{},
		Filters: qa.Predicates,
	}

	edgeMap := make(map[string]bool)

	for _, ji := range qa.JoinInfo {
		key := ji.LeftTable.Name + "-" + ji.RightTable.Name
		if !edgeMap[key] {
			edge := types.JoinEdge{
				LeftTable:   ji.LeftTable,
				RightTable:  ji.RightTable,
				Columns:     ji.Columns,
				Selectivity: 0.1,
			}
			graph.Edges = append(graph.Edges, edge)
			edgeMap[key] = true
		}
	}

	sort.Slice(graph.Edges, func(i, j int) bool {
		return graph.Edges[i].Selectivity < graph.Edges[j].Selectivity
	})

	return graph
}

func (o *JoinOrderOptimizer) GetPolicy() *Policy {
	return o.policy
}
