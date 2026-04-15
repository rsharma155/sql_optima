// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Derive bottleneck classification and high-level diagnostics from plan metrics.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_plandiagnostics

import (
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/pg_planmetrics"
	"github.com/rsharma155/sql_optima/internal/explain/pg_planparser"
)

type Bottleneck struct {
	Primary string `json:"primary"`
	Reason  string `json:"reason,omitempty"`
}

// ClassifyBottleneck implements the resource classification engine from enhance_analyze.md.
func ClassifyBottleneck(nodes []pg_planparser.FlatNode, totalExecMs float64, cat []pg_planmetrics.CategoryBreakdown) Bottleneck {
	byCat := map[string]float64{}
	for _, c := range cat {
		byCat[c.Category] = c.TimePercent
	}

	hasDiskSort := false
	hasTempWrites := false
	hasHashBatches := false
	hasGather := false
	var gatherPct float64
	var workersPlanned, workersLaunched int
	var maxRows int64
	for _, n := range nodes {
		if n.RowsProcessed > maxRows {
			maxRows = n.RowsProcessed
		}
		if strings.EqualFold(n.SortSpaceType, "Disk") {
			hasDiskSort = true
		}
		if n.TempWrittenBlocks > 0 {
			hasTempWrites = true
		}
		if n.HashBatches > 1 {
			hasHashBatches = true
		}
		if strings.Contains(strings.ToLower(n.NodeType), "gather") {
			hasGather = true
			gatherPct += pg_planmetrics.NodeTimePercent(n.NodeExecutionTime, totalExecMs)
		}
		if n.WorkersPlanned > workersPlanned {
			workersPlanned = n.WorkersPlanned
		}
		if n.WorkersLaunched > workersLaunched {
			workersLaunched = n.WorkersLaunched
		}
	}

	// Sort-bound: >30% sort OR any disk sort
	if byCat["Sort"] > 30 || hasDiskSort {
		return Bottleneck{Primary: "sort", Reason: "Sort operators consume significant runtime or spilled to disk"}
	}
	// Join-bound: join nodes >40%
	joinPct := byCat["Hash Join"] + byCat["Merge Join"] + byCat["Nested Loop"]
	if joinPct > 40 {
		return Bottleneck{Primary: "join", Reason: "Join operators dominate runtime"}
	}
	// Scan-bound: seq/index scans >40%
	scanPct := byCat["Seq Scan"] + byCat["Index Scan"] + byCat["Bitmap Heap Scan"] + byCat["Scan"]
	if scanPct > 40 {
		return Bottleneck{Primary: "scan", Reason: "Scan operators dominate runtime"}
	}
	// Memory pressure
	if hasTempWrites || hasDiskSort || hasHashBatches {
		return Bottleneck{Primary: "memory", Reason: "Disk spill signals detected (temp writes, disk sort, or hash batches)"}
	}
	// Parallel inefficiency
	if (workersPlanned > 0 && workersLaunched > 0 && workersPlanned > workersLaunched) || gatherPct > 20 {
		return Bottleneck{Primary: "parallel", Reason: "Parallel workers were planned but not fully launched or Gather is expensive"}
	}
	// Parallelism not used
	if !hasGather && maxRows >= 100000 {
		return Bottleneck{Primary: "parallel", Reason: "Large row volume without Gather suggests missing parallelism opportunity"}
	}
	return Bottleneck{Primary: "mixed", Reason: "No single dominant bottleneck detected"}
}

