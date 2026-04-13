package candidate

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/missing_index/catalog"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

func TestGenerateCandidatesBasic(t *testing.T) {
	queryAnalysis := &types.QueryAnalysis{
		Tables: []types.TableRef{{Schema: "public", Name: "orders"}},
		Predicates: []types.Predicate{
			{Table: types.TableRef{Schema: "public", Name: "orders"}, Column: "tenant_id", Type: types.PredicateTypeEquality},
			{Table: types.TableRef{Schema: "public", Name: "orders"}, Column: "status", Type: types.PredicateTypeEquality},
		},
		OrderBy: []types.OrderByColumn{
			{Table: types.TableRef{Schema: "public", Name: "orders"}, Column: "created_at", Descending: true},
		},
		ProjectedCols: []string{"id", "created_at"},
	}

	planAnalysis := &types.PlanAnalysis{
		TargetTables: []types.TableRef{{Schema: "public", Name: "orders"}},
		Opportunities: []types.TableOpportunity{
			{
				Table:       types.TableRef{Schema: "public", Name: "orders"},
				ScanType:    "Seq Scan",
				CurrentCost: 5000.0,
			},
		},
	}

	tableInfos := []catalog.TableInfo{
		{
			Table:    types.TableRef{Schema: "public", Name: "orders"},
			RowCount: 100000,
		},
	}

	options := &types.RequestOptions{
		MaxCandidates:  5,
		IncludeColumns: true,
	}

	candidates := GenerateCandidates(nil, queryAnalysis, planAnalysis, tableInfos, options)

	if len(candidates) == 0 {
		t.Error("Expected candidates to be generated")
	}
}

func TestBuildCandidateForTable(t *testing.T) {
	table := types.TableRef{Schema: "public", Name: "orders"}
	predicateCols := []string{"tenant_id", "status"}
	joinCols := []string{}
	orderCols := []string{"created_at"}
	tableInfo := &catalog.TableInfo{Table: table}
	options := &types.RequestOptions{IncludeColumns: true}

	candidate := buildCandidateForTable(table, predicateCols, joinCols, orderCols, tableInfo, options)

	if len(candidate.KeyColumns) == 0 {
		t.Error("Expected key columns to be set")
	}
}

func TestIsValidCandidate(t *testing.T) {
	valid := types.IndexCandidate{
		KeyColumns: []types.IndexColumn{{Name: "id"}},
	}

	if !isValidCandidate(valid) {
		t.Error("Expected candidate to be valid")
	}

	invalid := types.IndexCandidate{
		KeyColumns: []types.IndexColumn{},
	}

	if isValidCandidate(invalid) {
		t.Error("Expected candidate to be invalid")
	}
}

func TestContainsColumn(t *testing.T) {
	cols := []types.IndexColumn{
		{Name: "id"},
		{Name: "name"},
	}

	if !containsColumn(cols, "id") {
		t.Error("Expected id to be found")
	}

	if containsColumn(cols, "email") {
		t.Error("Expected email to not be found")
	}
}
