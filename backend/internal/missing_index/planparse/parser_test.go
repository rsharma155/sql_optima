// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Test suite for plan parser.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package planparse

import (
	"context"
	"testing"

	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

func TestParseBasicSeqScan(t *testing.T) {
	planJSON := map[string]any{
		"Node Type":     "Seq Scan",
		"Relation Name": "orders",
		"Schema":        "public",
		"Total Cost":    1500.0,
		"Plan Rows":     10000,
		"Filter":        "tenant_id = 123",
	}

	analysis, err := Parse(context.TODO(), planJSON)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if analysis == nil {
		t.Fatal("Analysis should not be nil")
	}

	if len(analysis.TargetTables) == 0 {
		t.Error("Expected target tables to be extracted")
	}

	if analysis.TargetTables[0].Name != "orders" {
		t.Errorf("Expected table 'orders', got %s", analysis.TargetTables[0].Name)
	}
}

func TestParseWithNestedPlans(t *testing.T) {
	planJSON := map[string]any{
		"Node Type":  "Sort",
		"Total Cost": 2000.0,
		"Plans": []any{
			map[string]any{
				"Node Type":     "Seq Scan",
				"Relation Name": "orders",
				"Schema":        "public",
				"Total Cost":    1500.0,
			},
		},
	}

	analysis, err := Parse(context.TODO(), planJSON)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if analysis == nil {
		t.Fatal("Analysis should not be nil")
	}
}

func TestParseEmptyPlan(t *testing.T) {
	_, err := Parse(context.TODO(), nil)
	if err != nil {
		t.Errorf("Expected no error for nil plan: %v", err)
	}
}

func TestExtractTargetTables(t *testing.T) {
	node := &types.PlanNode{
		NodeType:     "Seq Scan",
		RelationName: stringPtr("users"),
		Schema:       stringPtr("public"),
		Children: []*types.PlanNode{
			{
				NodeType:     "Index Scan",
				RelationName: stringPtr("orders"),
				Schema:       stringPtr("public"),
			},
		},
	}

	var tables []types.TableRef
	extractTargetTables(node, &tables)

	if len(tables) != 2 {
		t.Errorf("Expected 2 tables, got %d", len(tables))
	}
}

func TestIdentifyOpportunities(t *testing.T) {
	node := &types.PlanNode{
		NodeType:     "Seq Scan",
		RelationName: stringPtr("orders"),
		Schema:       stringPtr("public"),
		TotalCost:    5000.0,
		PlanRows:     100000,
		Filter:       stringPtr("tenant_id = 1"),
	}

	var opportunities []types.TableOpportunity
	identifyOpportunities(node, &opportunities)

	if len(opportunities) != 1 {
		t.Errorf("Expected 1 opportunity, got %d", len(opportunities))
	}

	if opportunities[0].CurrentCost != 5000.0 {
		t.Errorf("Expected cost 5000, got %f", opportunities[0].CurrentCost)
	}
}

func stringPtr(s string) *string {
	return &s
}
