// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for deterministic plan flattening and hashing.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_planparser

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/parser"
)

func TestPlanHash_DeterministicAcrossKeyOrder(t *testing.T) {
	j1 := []byte(`{"Plan":{"Node Type":"Seq Scan","Total Cost":10,"Plans":[{"Node Type":"Index Scan","Total Cost":1}]}, "Execution Time": 12.3, "Planning Time": 0.1}`)
	j2 := []byte(`{"Planning Time":0.1,"Execution Time":12.3,"Plan":{"Plans":[{"Total Cost":1,"Node Type":"Index Scan"}],"Total Cost":10,"Node Type":"Seq Scan"}}`)
	h1, err := PlanHash(j1)
	if err != nil {
		t.Fatalf("PlanHash j1: %v", err)
	}
	h2, err := PlanHash(j2)
	if err != nil {
		t.Fatalf("PlanHash j2: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected stable hash; got %s vs %s", h1, h2)
	}
}

func TestFlattenPlan_DerivedFields(t *testing.T) {
	// Build a minimal JSON plan in the existing parser format then flatten.
	raw := `[
	  {
		"Plan": {
		  "Node Type": "Seq Scan",
		  "Relation Name": "users",
		  "Actual Total Time": 5.0,
		  "Actual Rows": 10,
		  "Actual Loops": 3,
		  "Plans": [
			{ "Node Type": "Sort", "Actual Total Time": 1.0, "Actual Rows": 10, "Actual Loops": 3, "Sort Method":"external merge", "Sort Space Type":"Disk" }
		  ]
		},
		"Planning Time": 0.5,
		"Execution Time": 15.0
	  }
	]`
	pl, err := parser.ParseJSON(raw)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	nodes := FlattenPlan(pl)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// Root node: 5ms * 3 loops = 15ms, rows 10*3=30.
	if nodes[0].DepthLevel != 0 {
		t.Fatalf("expected root depth 0, got %d", nodes[0].DepthLevel)
	}
	if nodes[0].NodeExecutionTime != 15.0 {
		t.Fatalf("expected 15ms node execution time, got %v", nodes[0].NodeExecutionTime)
	}
	if nodes[0].RowsProcessed != 30 {
		t.Fatalf("expected 30 rows processed, got %v", nodes[0].RowsProcessed)
	}

	// Child sort node should carry disk sort signals.
	if nodes[1].DepthLevel != 1 {
		t.Fatalf("expected child depth 1, got %d", nodes[1].DepthLevel)
	}
	if nodes[1].SortSpaceType != "Disk" {
		t.Fatalf("expected sort space type Disk, got %q", nodes[1].SortSpaceType)
	}
	if nodes[1].SortMethod == "" {
		t.Fatalf("expected sort method")
	}
}

