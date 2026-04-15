// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for global metrics and cardinality estimation.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_planmetrics

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/pg_planparser"
)

func TestCardinalityClassification(t *testing.T) {
	nodes := []pg_planparser.FlatNode{
		{NodeID: 1, NodeType: "Seq Scan", PlanRows: 100, ActualRows: 120},
		{NodeID: 2, NodeType: "Hash Join", PlanRows: 100, ActualRows: 500},
		{NodeID: 3, NodeType: "Index Scan", PlanRows: 1000, ActualRows: 10},
	}
	c := Cardinality(nodes)
	if c.WorstEstimationNode == nil {
		t.Fatalf("expected worst node")
	}
	if c.WorstEstimationNode.NodeID != 3 {
		t.Fatalf("expected node 3 worst (0.01 severe), got %d", c.WorstEstimationNode.NodeID)
	}
	if c.WorstEstimationNode.Classification != "severe" {
		t.Fatalf("expected severe, got %q", c.WorstEstimationNode.Classification)
	}
}

func TestCategoryTimeAttribution_Buckets(t *testing.T) {
	nodes := []pg_planparser.FlatNode{
		{NodeID: 1, NodeType: "Sort", NodeExecutionTime: 40},
		{NodeID: 2, NodeType: "Hash Join", NodeExecutionTime: 60},
	}
	bd := CategoryTimeAttribution(nodes, 100)
	if len(bd) < 2 {
		t.Fatalf("expected multiple categories, got %d", len(bd))
	}
	if bd[0].TimeMs != 60 {
		t.Fatalf("expected hash join top, got %v", bd[0].TimeMs)
	}
}

