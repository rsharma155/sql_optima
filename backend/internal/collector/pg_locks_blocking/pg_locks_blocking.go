// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: PostgreSQL locks & blocking "stateful incident" collector helpers.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_locks_blocking

import (
	"sort"
)

// Pair represents a directed edge: blocking_pid -> blocked_pid.
// (i.e., blocking_pid is holding a lock that blocked_pid is waiting on).
type Pair struct {
	BlockedPID  int
	BlockingPID int
}

// GraphStats summarizes a blocking dependency graph for a single capture.
type GraphStats struct {
	VictimsDistinct int // distinct blocked pids
	ChainDepth      int // max depth across all roots
	RootBlockers    []int
}

// AnalyzePairs computes roots and max depth for a set of blocking pairs.
// Root blockers are blockers that are not themselves blocked (in this pair set).
func AnalyzePairs(pairs []Pair) GraphStats {
	if len(pairs) == 0 {
		return GraphStats{}
	}

	blockedSet := map[int]struct{}{}
	blockerSet := map[int]struct{}{}
	children := map[int][]int{} // blocker -> blocked list
	victims := map[int]struct{}{}

	for _, p := range pairs {
		if p.BlockingPID == 0 || p.BlockedPID == 0 {
			continue
		}
		blockedSet[p.BlockedPID] = struct{}{}
		blockerSet[p.BlockingPID] = struct{}{}
		children[p.BlockingPID] = append(children[p.BlockingPID], p.BlockedPID)
		victims[p.BlockedPID] = struct{}{}
	}

	roots := make([]int, 0)
	for b := range blockerSet {
		if _, isBlocked := blockedSet[b]; !isBlocked {
			roots = append(roots, b)
		}
	}
	sort.Ints(roots)

	// Depth-first depth computation with cycle guards.
	maxDepth := 0
	var dfs func(pid int, seen map[int]struct{}) int
	dfs = func(pid int, seen map[int]struct{}) int {
		if _, ok := seen[pid]; ok {
			// cycle: stop here
			return 0
		}
		seen2 := make(map[int]struct{}, len(seen)+1)
		for k := range seen {
			seen2[k] = struct{}{}
		}
		seen2[pid] = struct{}{}

		next := children[pid]
		if len(next) == 0 {
			return 1
		}
		best := 1
		for _, ch := range next {
			d := 1 + dfs(ch, seen2)
			if d > best {
				best = d
			}
		}
		return best
	}

	for _, r := range roots {
		d := dfs(r, map[int]struct{}{})
		if d > maxDepth {
			maxDepth = d
		}
	}
	if maxDepth == 0 && len(pairs) > 0 {
		maxDepth = 1
	}

	return GraphStats{
		VictimsDistinct: len(victims),
		ChainDepth:      maxDepth,
		RootBlockers:    roots,
	}
}

