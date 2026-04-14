// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for scorer.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package scoring

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/missing_index/catalog"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

func TestScoreCandidates(t *testing.T) {
	verificationResults := []types.VerificationResult{
		{
			Candidate: types.IndexCandidate{
				Table:       types.TableRef{Schema: "public", Name: "orders"},
				IndexMethod: types.IndexMethodBTree,
				KeyColumns:  []types.IndexColumn{{Name: "tenant_id"}},
			},
			OriginalCost:     5000.0,
			HypotheticalCost: 1500.0,
			ImprovementPct:   70.0,
			IndexUsedInPlan:  true,
			PlanChanged:      true,
			Success:          true,
		},
	}

	tableInfos := []catalog.TableInfo{
		{
			Table:    types.TableRef{Schema: "public", Name: "orders"},
			RowCount: 100000,
		},
	}

	options := &types.RequestOptions{
		MinImprovementPct: 15.0,
	}

	scored := ScoreCandidates(nil, verificationResults, tableInfos, options)

	if len(scored) == 0 {
		t.Error("Expected scored candidates to be returned")
	}

	if scored[0].Confidence < 0.65 {
		t.Errorf("Expected confidence >= 0.65, got %f", scored[0].Confidence)
	}
}

func TestCalculateConfidence(t *testing.T) {
	result := types.VerificationResult{
		Candidate: types.IndexCandidate{
			KeyColumns: []types.IndexColumn{
				{Name: "tenant_id"},
				{Name: "status"},
			},
		},
		ImprovementPct:  70.0,
		IndexUsedInPlan: true,
		PlanChanged:     true,
	}

	confidence := calculateConfidence(result, nil)

	if confidence < 0.65 {
		t.Errorf("Expected confidence >= 0.65, got %f", confidence)
	}
}

func TestBuildIndexStatement(t *testing.T) {
	candidate := types.IndexCandidate{
		Table:       types.TableRef{Schema: "public", Name: "orders"},
		IndexMethod: types.IndexMethodBTree,
		KeyColumns: []types.IndexColumn{
			{Name: "tenant_id"},
			{Name: "status", Descending: true},
		},
		IncludeCols: []types.IndexColumn{
			{Name: "id"},
		},
	}

	stmt := buildIndexStatement(candidate)

	if stmt == "" {
		t.Error("Expected index statement to be generated")
	}

	if len(stmt) == 0 {
		t.Error("Statement should not be empty")
	}
}

func TestBuildReasoning(t *testing.T) {
	result := types.VerificationResult{
		ImprovementPct:  50.0,
		IndexUsedInPlan: true,
		PlanChanged:     true,
	}

	reasoning := buildReasoning(result, 0.85)

	if len(reasoning) == 0 {
		t.Error("Expected reasoning to be generated")
	}
}
