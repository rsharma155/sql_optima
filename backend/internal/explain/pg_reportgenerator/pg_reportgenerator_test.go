// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Unit tests for deterministic report generation pipeline.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_reportgenerator

import (
	"testing"

	"github.com/rsharma155/sql_optima/internal/explain/parser"
)

func TestGenerate_ReportLayoutContract(t *testing.T) {
	raw := `[
	  {
		"Plan": {
		  "Node Type": "Sort",
		  "Actual Total Time": 10.0,
		  "Actual Rows": 100,
		  "Actual Loops": 1,
		  "Sort Method": "external merge",
		  "Sort Space Type": "Disk",
		  "Plans": [
			{ "Node Type": "Seq Scan", "Relation Name":"t", "Actual Total Time": 40.0, "Actual Rows": 200000, "Actual Loops": 1, "Filter":"(x = 1)" }
		  ]
		},
		"Planning Time": 0.2,
		"Execution Time": 60.0
	  }
	]`
	pl, err := parser.ParseJSON(raw)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	rep := Generate(pl, DefaultOptions())
	if rep == nil {
		t.Fatalf("expected report")
	}
	if len(rep.ExecutionSummary) != 1 {
		t.Fatalf("expected 1-row execution_summary, got %d", len(rep.ExecutionSummary))
	}
	if rep.ExecutionSummary[0].ExecutionTimeMs != 60 {
		t.Fatalf("execution time")
	}
	if len(rep.TimeBreakdown) == 0 {
		t.Fatalf("expected time_breakdown")
	}
	if len(rep.TopNodes) == 0 {
		t.Fatalf("expected top_nodes")
	}
	if len(rep.MemoryDisk) != 1 {
		t.Fatalf("expected memory_disk single row")
	}
	if !rep.MemoryDisk[0].DiskSortDetected {
		t.Fatalf("expected disk_sort_detected")
	}
	// Hash spill only when hash_batches > 1 on a node; this fixture has none.
	if len(rep.Findings) == 0 {
		t.Fatalf("expected findings")
	}
	found := false
	for _, f := range rep.Findings {
		if f.FindingCode == "disk_sort" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected disk_sort finding row")
	}
	if len(rep.IndexOpportunities) == 0 {
		t.Fatalf("expected index opportunities for seq scan + filter")
	}
	if len(rep.TuningRecommendations) == 0 {
		t.Fatalf("expected tuning recommendations")
	}
}
