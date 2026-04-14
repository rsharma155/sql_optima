// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Index candidate builder from query analysis.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package candidate

import (
	"context"
	"sort"
	"strings"

	"github.com/rsharma155/sql_optima/internal/missing_index/catalog"
	"github.com/rsharma155/sql_optima/internal/missing_index/logging"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

type ColumnType int

const (
	ColumnTypeUnknown ColumnType = iota
	ColumnTypeNumeric
	ColumnTypeInteger
	ColumnTypeDate
	ColumnTypeTimestamp
	ColumnTypeText
	ColumnTypeVarchar
	ColumnTypeBoolean
)

// GenerateCandidates creates potential index candidates based on query analysis
func GenerateCandidates(
	ctx context.Context,
	queryAnalysis *types.QueryAnalysis,
	planAnalysis *types.PlanAnalysis,
	tableInfos []catalog.TableInfo,
	options *types.RequestOptions,
) []types.IndexCandidate {

	if queryAnalysis == nil || planAnalysis == nil {
		logging.Info(ctx, "No query or plan analysis available", nil)
		return []types.IndexCandidate{}
	}

	if options == nil {
		options = defaultOptions()
	}

	candidates := []types.IndexCandidate{}
	maxCandidates := options.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 5
	}

	existingIndexMap := buildExistingIndexMap(tableInfos)

	if len(planAnalysis.Opportunities) == 0 && len(queryAnalysis.Tables) > 0 {
		logging.Info(ctx, "No opportunities, generating from query tables", map[string]any{
			"table_count": len(queryAnalysis.Tables),
		})
		for _, table := range queryAnalysis.Tables {
			opp := types.TableOpportunity{
				Table:       table,
				ScanType:    "Seq Scan",
				CurrentCost: 10000,
			}
			planAnalysis.Opportunities = append(planAnalysis.Opportunities, opp)
		}
	}

	for _, opp := range planAnalysis.Opportunities {
		tableInfo := findTableInfo(tableInfos, opp.Table)

		predicateCols := getPredicateColumnsForTable(queryAnalysis, opp.Table)
		joinCols := getJoinColumnsForTable(queryAnalysis, opp.Table)
		orderCols := getOrderByColumnsForTable(queryAnalysis, opp.Table)

		if len(predicateCols) == 0 && len(joinCols) == 0 && len(orderCols) == 0 {
			predicateCols = opp.FilterColumns
			joinCols = opp.JoinColumns
			orderCols = opp.OrderByColumns
		}

		candidate := buildCandidateForTable(opp.Table, predicateCols, joinCols, orderCols, tableInfo, options)

		if isDuplicateCandidate(candidate, existingIndexMap, candidates) {
			logging.Debug(ctx, "Skipping duplicate candidate", map[string]any{"table": opp.Table.Name})
			continue
		}

		if isValidCandidate(candidate) {
			candidates = append(candidates, candidate)

			if len(predicateCols) > 1 {
				for i := 1; i < len(predicateCols) && len(candidates) < maxCandidates; i++ {
					subCandidate := buildCandidateForTable(
						opp.Table,
						predicateCols[:i+1],
						joinCols,
						orderCols,
						tableInfo,
						options,
					)
					if isValidCandidate(subCandidate) && !isDuplicateCandidate(subCandidate, existingIndexMap, candidates) {
						candidates = append(candidates, subCandidate)
					}
				}
			}

			if options.IncludeColumns {
				includeCandidate := addIncludeColumns(candidate, queryAnalysis.ProjectedCols)
				if isValidCandidate(includeCandidate) && !isDuplicateCandidate(includeCandidate, existingIndexMap, candidates) {
					candidates = append(candidates, includeCandidate)
				}
			}
		}

		if len(candidates) >= maxCandidates {
			break
		}
	}

	candidates = rankCandidates(ctx, candidates, planAnalysis)

	logging.Info(ctx, "Generated candidates", map[string]any{
		"count": len(candidates),
	})

	return candidates
}

func defaultOptions() *types.RequestOptions {
	return &types.RequestOptions{
		MaxCandidates:     5,
		MinImprovementPct: 15.0,
		IncludeColumns:    true,
	}
}

func findTableInfo(infos []catalog.TableInfo, table types.TableRef) *catalog.TableInfo {
	for i := range infos {
		if infos[i].Table.Schema == table.Schema && infos[i].Table.Name == table.Name {
			return &infos[i]
		}
	}
	return nil
}

func getPredicateColumnsForTable(qa *types.QueryAnalysis, table types.TableRef) []string {
	return getOrderedPredicateColumnsForTable(qa, table)
}

func predicateOrdering(t types.PredicateType) int {
	switch t {
	case types.PredicateTypeEquality:
		return 0
	case types.PredicateTypeIsNull:
		return 1
	case types.PredicateTypeIn:
		return 2
	case types.PredicateTypeRange:
		return 3
	default:
		return 4
	}
}

func getOrderedPredicateColumnsForTable(qa *types.QueryAnalysis, table types.TableRef) []string {
	if qa == nil {
		return nil
	}
	colRank := make(map[string]int)
	for _, pred := range qa.Predicates {
		if pred.Column == "" || pred.Table.Name != table.Name {
			continue
		}
		if pred.Table.Schema != table.Schema && pred.Table.Schema != "" {
			continue
		}
		r := predicateOrdering(pred.Type)
		if prev, ok := colRank[pred.Column]; !ok || r < prev {
			colRank[pred.Column] = r
		}
	}
	type pair struct {
		name string
		r    int
	}
	var pairs []pair
	for n, r := range colRank {
		pairs = append(pairs, pair{n, r})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].r != pairs[j].r {
			return pairs[i].r < pairs[j].r
		}
		return pairs[i].name < pairs[j].name
	})
	out := make([]string, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.name)
	}
	return out
}

func getJoinColumnsForTable(qa *types.QueryAnalysis, table types.TableRef) []string {
	cols := []string{}
	for _, join := range qa.JoinInfo {
		if join.LeftTable.Name == table.Name || join.RightTable.Name == table.Name {
			for _, col := range join.Columns {
				if join.LeftTable.Name == table.Name {
					cols = append(cols, col.LeftCol)
				}
				if join.RightTable.Name == table.Name {
					cols = append(cols, col.RightCol)
				}
			}
		}
	}
	return cols
}

func getOrderByColumnsForTable(qa *types.QueryAnalysis, table types.TableRef) []string {
	cols := []string{}
	single := len(qa.Tables) == 1
	for _, order := range qa.OrderBy {
		if order.Column == "" {
			continue
		}
		if order.Table.Name == "" && single {
			cols = append(cols, order.Column)
			continue
		}
		if order.Table.Name == table.Name && (order.Table.Schema == table.Schema || order.Table.Schema == "") {
			cols = append(cols, order.Column)
		}
	}
	return cols
}

type scoredColumn struct {
	name     string
	priority int
	predType types.PredicateType
}

func buildCandidateForTable(
	table types.TableRef,
	predicateCols []string,
	joinCols []string,
	orderCols []string,
	tableInfo *catalog.TableInfo,
	options *types.RequestOptions,
) types.IndexCandidate {

	keyCols := []types.IndexColumn{}
	reasoning := []string{}

	scoredCols := []scoredColumn{}

	for _, col := range predicateCols {
		colType := classifyColumnType(col, tableInfo)
		priority := getColumnPriority(colType)
		scoredCols = append(scoredCols, scoredColumn{name: col, priority: priority, predType: types.PredicateTypeEquality})
	}

	for _, col := range joinCols {
		exists := false
		for _, sc := range scoredCols {
			if sc.name == col {
				exists = true
				break
			}
		}
		if !exists {
			colType := classifyColumnType(col, tableInfo)
			priority := getColumnPriority(colType)
			scoredCols = append(scoredCols, scoredColumn{name: col, priority: priority, predType: types.PredicateTypeJoin})
		}
	}

	for _, col := range orderCols {
		exists := false
		for _, sc := range scoredCols {
			if sc.name == col {
				exists = true
				break
			}
		}
		if !exists {
			colType := classifyColumnType(col, tableInfo)
			priority := getColumnPriority(colType)
			scoredCols = append(scoredCols, scoredColumn{name: col, priority: priority, predType: types.PredicateTypeRange})
		}
	}

	sort.Slice(scoredCols, func(i, j int) bool {
		if scoredCols[i].priority != scoredCols[j].priority {
			return scoredCols[i].priority < scoredCols[j].priority
		}
		if scoredCols[i].predType == types.PredicateTypeEquality && scoredCols[j].predType != types.PredicateTypeEquality {
			return true
		}
		return false
	})

	for _, sc := range scoredCols {
		if len(keyCols) < 5 {
			keyCols = append(keyCols, types.IndexColumn{Name: sc.name})
		}
	}

	reasoning = append(reasoning, "Equality predicates on "+table.Name)
	reasoning = append(reasoning, "Table has high-cost scan in execution plan")

	if len(joinCols) > 0 {
		reasoning = append(reasoning, "Join predicates benefit from index")
	}

	if len(orderCols) > 0 {
		reasoning = append(reasoning, "ORDER BY columns included for index-only sort")
	}

	return types.IndexCandidate{
		Table:       table,
		IndexMethod: types.IndexMethodBTree,
		KeyColumns:  keyCols,
		IncludeCols: []types.IndexColumn{},
		Reasoning:   reasoning,
		Score:       float64(len(keyCols)) * 10,
	}
}

func containsColumn(cols []types.IndexColumn, name string) bool {
	for _, c := range cols {
		if c.Name == name {
			return true
		}
	}
	return false
}

func isValidCandidate(candidate types.IndexCandidate) bool {
	return len(candidate.KeyColumns) > 0
}

func addIncludeColumns(candidate types.IndexCandidate, projectedCols []string) types.IndexCandidate {
	includeCols := []types.IndexColumn{}

	for _, col := range projectedCols {
		if !containsColumn(candidate.KeyColumns, col) && len(includeCols) < 3 {
			includeCols = append(includeCols, types.IndexColumn{Name: col})
		}
	}

	candidate.IncludeCols = includeCols
	candidate.Reasoning = append(candidate.Reasoning, "Added projected columns as INCLUDE for index-only scans")

	return candidate
}

func rankCandidates(ctx context.Context, candidates []types.IndexCandidate, planAnalysis *types.PlanAnalysis) []types.IndexCandidate {
	scored := make([]types.IndexCandidate, len(candidates))
	copy(scored, candidates)

	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].Score > scored[i].Score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	return scored
}

func buildExistingIndexMap(tableInfos []catalog.TableInfo) map[string][]string {
	indexMap := make(map[string][]string)
	for _, ti := range tableInfos {
		key := ti.Table.Schema + "." + ti.Table.Name
		for _, idx := range ti.IndexStats {
			indexMap[key] = append(indexMap[key], idx.IndexName)
		}
	}
	return indexMap
}

func isDuplicateCandidate(candidate types.IndexCandidate, existingIndexMap map[string][]string, candidates []types.IndexCandidate) bool {
	key := candidate.Table.Schema + "." + candidate.Table.Name
	existingIndexes := existingIndexMap[key]

	for _, c := range candidates {
		if c.Table.Schema == candidate.Table.Schema && c.Table.Name == candidate.Table.Name {
			if len(c.KeyColumns) == len(candidate.KeyColumns) {
				match := true
				for i, kc := range c.KeyColumns {
					if kc.Name != candidate.KeyColumns[i].Name {
						match = false
						break
					}
				}
				if match {
					return true
				}
			}
		}
	}

	_ = existingIndexes
	return false
}

const (
	MaxCombinedCandidates  = 20
	MaxIndexesPerCandidate = 2
)

type CombinedCandidateBuilder struct {
	queryVariants   []QueryVariant
	indexCandidates []types.IndexCandidate
}

type QueryVariant struct {
	Query        string
	AppliedRules []string
}

func GenerateCombinedCandidates(
	ctx context.Context,
	queryVariants []QueryVariant,
	indexCandidates []types.IndexCandidate,
) []types.CombinedCandidate {

	if len(queryVariants) == 0 || len(indexCandidates) == 0 {
		return []types.CombinedCandidate{}
	}

	var combined []types.CombinedCandidate

	for _, qv := range queryVariants {
		for _, ic := range indexCandidates {
			if len(combined) >= MaxCombinedCandidates {
				break
			}

			indexes := []types.IndexCandidate{ic}
			if len(indexCandidates) > 1 {
				for i := 0; i < len(indexCandidates) && i < MaxIndexesPerCandidate; i++ {
					if indexCandidates[i].Table.Name == ic.Table.Name {
						indexes = append(indexes, indexCandidates[i])
					}
				}
			}

			combined = append(combined, types.CombinedCandidate{
				Query:    qv.Query,
				Indexes:  indexes,
				Rewrites: qv.AppliedRules,
			})
		}

		if len(combined) >= MaxCombinedCandidates {
			break
		}
	}

	logging.Info(ctx, "Generated combined candidates", map[string]any{
		"count":            len(combined),
		"query_variants":   len(queryVariants),
		"index_candidates": len(indexCandidates),
	})

	return combined
}

func classifyColumnType(colName string, tableInfo *catalog.TableInfo) ColumnType {
	if tableInfo == nil {
		return ColumnTypeUnknown
	}
	lowerName := strings.ToLower(colName)
	for _, col := range tableInfo.ColumnStats {
		if strings.ToLower(col.ColumnName) == lowerName {
			return inferColumnType(col)
		}
	}
	return ColumnTypeUnknown
}

func inferColumnType(colStats types.ColumnStats) ColumnType {
	if colStats.NDistinct != nil {
		if *colStats.NDistinct <= 5 {
			return ColumnTypeInteger
		}
		if *colStats.NDistinct <= 100 {
			return ColumnTypeInteger
		}
	}
	return ColumnTypeUnknown
}

func getColumnPriority(colType ColumnType) int {
	switch colType {
	case ColumnTypeInteger, ColumnTypeNumeric:
		return 1
	case ColumnTypeDate, ColumnTypeTimestamp:
		return 2
	case ColumnTypeBoolean:
		return 3
	case ColumnTypeVarchar:
		return 4
	case ColumnTypeText:
		return 5
	default:
		return 99
	}
}
