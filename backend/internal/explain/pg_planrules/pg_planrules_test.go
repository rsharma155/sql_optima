// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for the EXPLAIN plan rules engine.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_planrules

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/pg_planmetrics"
	"github.com/rsharma155/sql_optima/internal/explain/pg_planparser"
)

func TestDetectFindings_DiskSortAndHashSpill(t *testing.T) {
	nodes := []pg_planparser.FlatNode{
		{NodeID: 1, NodeType: "Sort", NodeExecutionTime: 40, RowsProcessed: 200000, SortMethod: "external merge", SortSpaceType: "Disk"},
		{NodeID: 2, NodeType: "Hash Join", NodeExecutionTime: 60, RowsProcessed: 200000, HashBatches: 8},
	}
	card := pg_planmetrics.CardinalitySummary{}
	f := DetectFindings(nodes, 100, card, DefaultOptions())

	var hasDiskSort, hasHashSpill bool
	for _, x := range f {
		if x.Code == "disk_sort" {
			hasDiskSort = true
		}
		if x.Code == "hash_spill" {
			hasHashSpill = true
		}
	}
	if !hasDiskSort {
		t.Fatalf("expected disk_sort finding")
	}
	if !hasHashSpill {
		t.Fatalf("expected hash_spill finding")
	}
}

