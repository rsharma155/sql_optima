// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Parse/flatten PostgreSQL EXPLAIN (FORMAT JSON) plans into a deterministic, analysis-friendly shape.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package pg_planparser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

// FlatNode is a deterministic, flattened representation of a plan node.
// It intentionally stores both raw node fields (when available) and derived runtime metrics.
type FlatNode struct {
	// Basic metadata
	NodeID        int    `json:"node_id"`
	NodeType      string `json:"node_type"`
	RelationName  string `json:"relation_name,omitempty"`
	JoinType      string `json:"join_type,omitempty"`
	ParentNodeID  int    `json:"parent_node_id,omitempty"`
	DepthLevel    int    `json:"depth_level"`
	ParallelAware bool   `json:"parallel_aware,omitempty"`
	WorkersPlanned  int  `json:"workers_planned,omitempty"`
	WorkersLaunched int  `json:"workers_launched,omitempty"`

	// Cost & timing
	StartupCost       float64 `json:"startup_cost,omitempty"`
	TotalCost         float64 `json:"total_cost,omitempty"`
	PlanRows          int     `json:"plan_rows,omitempty"`
	PlanWidth         int     `json:"plan_width,omitempty"`
	ActualRows        int     `json:"actual_rows,omitempty"`
	ActualLoops       int     `json:"actual_loops,omitempty"`
	ActualStartupTime float64 `json:"actual_startup_time,omitempty"`
	ActualTotalTime   float64 `json:"actual_total_time,omitempty"`

	// Derived runtime (ms)
	NodeExecutionTime float64 `json:"node_execution_time_ms,omitempty"`
	RowsProcessed     int64   `json:"rows_processed,omitempty"`

	// Memory / disk signals (optional)
	SortMethod     string `json:"sort_method,omitempty"`
	SortSpaceUsed  int    `json:"sort_space_used,omitempty"`
	SortSpaceType  string `json:"sort_space_type,omitempty"`
	PeakMemoryUsage int   `json:"peak_memory_usage_kb,omitempty"`
	HashBuckets    int    `json:"hash_buckets,omitempty"`
	HashBatches    int    `json:"hash_batches,omitempty"`
	SharedHitBlocks   int `json:"shared_hit_blocks,omitempty"`
	SharedReadBlocks  int `json:"shared_read_blocks,omitempty"`
	TempReadBlocks    int `json:"temp_read_blocks,omitempty"`
	TempWrittenBlocks int `json:"temp_written_blocks,omitempty"`

	// Conditions (used for index heuristics)
	JoinCond  string   `json:"join_cond,omitempty"`
	HashCond  string   `json:"hash_cond,omitempty"`
	MergeCond string   `json:"merge_cond,omitempty"`
	GroupKey  []string `json:"group_key,omitempty"`
	SortKey   []string `json:"sort_key,omitempty"`
	Filter    string   `json:"filter,omitempty"`
	IndexCond string   `json:"index_cond,omitempty"`
}

// FlattenPlan recursively walks the plan tree and returns a deterministic list of FlatNodes.
func FlattenPlan(plan *types.Plan) []FlatNode {
	if plan == nil {
		return nil
	}

	var out []FlatNode

	var walk func(n *types.PlanNode, parentID, depth int)
	walk = func(n *types.PlanNode, parentID, depth int) {
		if n == nil {
			return
		}
		loops := n.ActualLoops
		if loops <= 0 {
			loops = 1
		}
		nodeExec := n.ActualTotalTime * float64(loops)
		rowsProcessed := int64(n.ActualRows) * int64(loops)

		fn := FlatNode{
			NodeID:         n.ID,
			NodeType:       strings.TrimSpace(n.NodeType),
			RelationName:   strings.TrimSpace(n.RelationName),
			JoinType:       strings.TrimSpace(n.JoinType),
			ParentNodeID:   parentID,
			DepthLevel:     depth,
			WorkersPlanned: n.WorkersPlanned,
			WorkersLaunched: n.WorkersLaunched,

			StartupCost:       n.StartupCost,
			TotalCost:         n.TotalCost,
			PlanRows:          n.PlanRows,
			PlanWidth:         n.PlanWidth,
			ActualRows:        n.ActualRows,
			ActualLoops:       n.ActualLoops,
			ActualStartupTime: n.ActualStartupTime,
			ActualTotalTime:   n.ActualTotalTime,

			NodeExecutionTime: nodeExec,
			RowsProcessed:     rowsProcessed,

			SortMethod:      strings.TrimSpace(n.SortMethod),
			SortSpaceUsed:   n.SortSpaceUsed,
			SortSpaceType:   strings.TrimSpace(n.SortSpaceType),
			HashBuckets:     n.Buckets,
			HashBatches:     n.Batches,
			Filter:          strings.TrimSpace(n.Filter),
			IndexCond:       strings.TrimSpace(n.IndexCond),
			HashCond:        strings.TrimSpace(n.HashCond),
			MergeCond:       strings.TrimSpace(n.MergeCond),
			GroupKey:        append([]string(nil), n.GroupKey...),
			SortKey:         append([]string(nil), n.SortKey...),
		}
		if n.Buffers != nil {
			fn.SharedHitBlocks = n.Buffers.SharedHit
			fn.SharedReadBlocks = n.Buffers.SharedRead
			fn.TempReadBlocks = n.Buffers.TempRead
			fn.TempWrittenBlocks = n.Buffers.TempWritten
		} else {
			// Fallback: some older normalizations flatten shared_* into plan node fields.
			fn.SharedHitBlocks = n.SharedHitBuffers
			fn.SharedReadBlocks = n.SharedReadBuffers
		}
		// Peak memory usage is normalized into MemoryUsage but the requirements want peak_memory_usage.
		fn.PeakMemoryUsage = n.MemoryUsage

		out = append(out, fn)

		for i := range n.Plans {
			walk(&n.Plans[i], n.ID, depth+1)
		}
	}

	root := plan.Plan
	walk(&root, 0, 0)

	// Deterministic ordering: depth-first traversal is stable, but ensure ties are stable if upstream changes.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DepthLevel != out[j].DepthLevel {
			return out[i].DepthLevel < out[j].DepthLevel
		}
		if out[i].ParentNodeID != out[j].ParentNodeID {
			return out[i].ParentNodeID < out[j].ParentNodeID
		}
		return out[i].NodeID < out[j].NodeID
	})

	// Re-assign deterministic node ids for analysis (do not depend on planner ids being present/non-zero).
	for i := range out {
		out[i].NodeID = i + 1
	}

	// Fix parent references after re-id.
	// We map by (depth, parent planner id order) is hard; instead: rebuild via traversal order assumptions:
	// parent is always earlier in DFS, and the closest earlier node with depth = current.depth-1 is the parent.
	// This works for well-formed trees and keeps deterministic links for reporting.
	stack := make([]int, 0, 16) // node ids by depth
	for i := range out {
		d := out[i].DepthLevel
		for len(stack) > d {
			stack = stack[:len(stack)-1]
		}
		if d == 0 {
			out[i].ParentNodeID = 0
		} else if d-1 < len(stack) {
			out[i].ParentNodeID = stack[d-1]
		}
		if len(stack) == d {
			stack = append(stack, out[i].NodeID)
		} else if len(stack) > d {
			stack[d] = out[i].NodeID
		}
	}

	return out
}

// CanonicalJSON marshals arbitrary JSON into a deterministic, key-sorted form.
func CanonicalJSON(raw []byte) ([]byte, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return marshalCanonical(v)
}

func marshalCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			b.Write(kb)
			b.WriteByte(':')
			vb, err := marshalCanonical(t[k])
			if err != nil {
				return nil, err
			}
			b.Write(vb)
		}
		b.WriteByte('}')
		return []byte(b.String()), nil
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for i := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			vb, err := marshalCanonical(t[i])
			if err != nil {
				return nil, err
			}
			b.Write(vb)
		}
		b.WriteByte(']')
		return []byte(b.String()), nil
	case json.Number:
		// Preserve integer-like numbers as integers when possible.
		s := strings.TrimSpace(t.String())
		if strings.ContainsAny(s, ".eE") {
			f, err := t.Float64()
			if err != nil {
				return json.Marshal(s)
			}
			return json.Marshal(f)
		}
		if i, err := t.Int64(); err == nil {
			return []byte(strconv.FormatInt(i, 10)), nil
		}
		f, err := t.Float64()
		if err != nil {
			return json.Marshal(s)
		}
		return json.Marshal(f)
	default:
		return json.Marshal(t)
	}
}

// PlanHash returns a stable hash for a JSON plan payload (canonical JSON SHA-256 hex).
func PlanHash(rawPlanJSON []byte) (string, error) {
	canon, err := CanonicalJSON(rawPlanJSON)
	if err != nil {
		return "", fmt.Errorf("canonicalize plan JSON: %w", err)
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

