// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: Insights generation from EXPLAIN results.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT
package explain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

const planHotCost = 1000.0
const planRowMismatchRatio = 5.0

// PlanHeuristicInsight is a plan-driven hint (JSON plan analysis); not the DB rules engine.
type PlanHeuristicInsight struct {
	Code            string `json:"code"`
	Severity        string `json:"severity"`
	Title           string `json:"title"`
	Message         string `json:"message"`
	NodeID          int    `json:"node_id,omitempty"`
	NodeType        string `json:"node_type,omitempty"`
	RelationName    string `json:"relation_name,omitempty"`
	SuggestedIndex  string `json:"suggested_index_sql,omitempty"`
	RewriteHint     string `json:"rewrite_hint,omitempty"`
}

// BuildHeuristicPlanInsights walks the parsed JSON plan (types.PlanNode) and emits H*-style hints
// aligned with enhance_plan_analyze.md (plan inspection only; no ruleengine DB).
func BuildHeuristicPlanInsights(root *types.PlanNode, query string) []PlanHeuristicInsight {
	if root == nil {
		return nil
	}
	var out []PlanHeuristicInsight
	var walk func(n *types.PlanNode, parent *types.PlanNode, childIdx int, nchild int)
	walk = func(n *types.PlanNode, parent *types.PlanNode, childIdx int, nchild int) {
		if n == nil {
			return
		}
		nt := n.NodeType
		rel := n.RelationName

		if strings.Contains(nt, "Seq Scan") && rel != "" && n.Filter != "" {
			if n.TotalCost >= planHotCost || rowMismatchHot(n) {
				ddl := suggestIndexDDL(n)
				out = append(out, PlanHeuristicInsight{
					Code:           "H1_SEQ_SCAN_FILTER",
					Severity:       "high",
					Title:          "Sequential scan with filter",
					Message:        fmt.Sprintf("Seq Scan on %s with filter — likely missing supporting index (est. cost %.0f).", rel, n.TotalCost),
					NodeID:         n.ID,
					NodeType:       nt,
					RelationName:   rel,
					SuggestedIndex: ddl,
					RewriteHint:    "Ensure predicates are sargable (no functions on indexed columns).",
				})
			}
		}

		if nt == "Sort" && len(n.SortKey) > 0 && (n.TotalCost >= 100 || rowMismatchHot(n)) {
			sm := strings.ToLower(strings.TrimSpace(n.SortMethod))
			if sm != "top-n heapsort" {
				out = append(out, PlanHeuristicInsight{
					Code:        "H2_SORT",
					Severity:    "medium",
					Title:       "Sort operation",
					Message:     fmt.Sprintf("Sort on %s — consider an index matching ORDER BY to avoid sorting.", strings.Join(n.SortKey, ", ")),
					NodeID:      n.ID,
					NodeType:    nt,
					RewriteHint: "Composite index: equality/WHERE columns first, then ORDER BY columns.",
				})
			}
		}

		if strings.Contains(nt, "Nested Loop") && len(n.Plans) >= 2 {
			inner := &n.Plans[len(n.Plans)-1]
			if inner != nil && strings.Contains(inner.NodeType, "Seq Scan") && inner.RelationName != "" {
				out = append(out, PlanHeuristicInsight{
					Code:           "H3_NL_SEQ_INNER",
					Severity:       "high",
					Title:          "Nested loop with sequential inner",
					Message:        fmt.Sprintf("Inner side is Seq Scan on %s — often fixed by an index on the join/filter column.", inner.RelationName),
					NodeID:         inner.ID,
					NodeType:       inner.NodeType,
					RelationName:   inner.RelationName,
					SuggestedIndex: suggestIndexDDL(inner),
					RewriteHint:    "Index the inner table on the column(s) used in the nested-loop join condition.",
				})
			}
		}

		if (nt == "Hash Join" || nt == "Merge Join") && n.TotalCost >= planHotCost {
			out = append(out, PlanHeuristicInsight{
				Code:          "H4_LARGE_JOIN",
				Severity:      "medium",
				Title:         "Large hash/merge join",
				Message:       fmt.Sprintf("%s with high cost (%.0f) — indexes on join keys can enable nested loop or reduce build size.", nt, n.TotalCost),
				NodeID:        n.ID,
				NodeType:      nt,
				RewriteHint:   "B-tree indexes on both sides of the join equality predicates.",
			})
		}

		if strings.Contains(nt, "Bitmap Heap Scan") && rel != "" {
			out = append(out, PlanHeuristicInsight{
				Code:           "H5_BITMAP_HEAP",
				Severity:       "medium",
				Title:          "Bitmap heap scan",
				Message:        fmt.Sprintf("Bitmap Heap Scan on %s — selective filters may benefit from a more selective composite index.", rel),
				NodeID:         n.ID,
				NodeType:       nt,
				RelationName:   rel,
				SuggestedIndex: suggestIndexDDL(n),
			})
		}

		if (strings.Contains(nt, "Aggregate") || strings.Contains(nt, "Group")) && n.PlanRows > 100000 {
			out = append(out, PlanHeuristicInsight{
				Code:         "H6_AGG_SCAN",
				Severity:     "medium",
				Title:        "Aggregation over many rows",
				Message:      "Large aggregated row estimate — consider an index on GROUP BY columns.",
				NodeID:       n.ID,
				NodeType:     nt,
				RewriteHint:  "Index leading columns matching GROUP BY can enable hash aggregate optimizations or reduce input.",
			})
		}

		kids := n.Plans
		for i := range kids {
			walk(&kids[i], n, i, len(kids))
		}
	}
	walk(root, nil, 0, 0)

	q := strings.TrimSpace(query)
	if q != "" {
		if matchesSelectStar(q) {
			out = append(out, PlanHeuristicInsight{
				Code:          "R1_SELECT_STAR",
				Severity:      "low",
				Title:         "SELECT *",
				Message:       "SELECT * prevents index-only scans and hides projection cost.",
				RewriteHint:   "List only required columns (see projected list from parser when available).",
			})
		}
		if matchesOffset(q) {
			out = append(out, PlanHeuristicInsight{
				Code:          "R2_OFFSET",
				Severity:      "medium",
				Title:         "OFFSET pagination",
				Message:       "OFFSET skips rows on each page — cost grows with page depth.",
				RewriteHint:   "Prefer keyset pagination: WHERE (sort_col, id) > ($1,$2) ORDER BY ... LIMIT.",
			})
		}
	}

	return dedupeInsights(out)
}

func dedupeInsights(in []PlanHeuristicInsight) []PlanHeuristicInsight {
	seen := make(map[string]struct{})
	var out []PlanHeuristicInsight
	for _, x := range in {
		key := x.Code + "|" + fmt.Sprintf("%d", x.NodeID) + "|" + x.RelationName + "|" + x.Title
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, x)
	}
	return out
}

func rowMismatchHot(n *types.PlanNode) bool {
	if n == nil || n.PlanRows <= 0 || n.ActualRows <= 0 {
		return false
	}
	r := float64(n.ActualRows) / float64(n.PlanRows)
	return r >= planRowMismatchRatio || r <= 1.0/planRowMismatchRatio
}

func matchesSelectStar(q string) bool {
	u := strings.ToUpper(q)
	if strings.Contains(u, "SELECT COUNT") {
		return false
	}
	return regexpSelectStar.MatchString(q)
}

func matchesOffset(q string) bool {
	return regexpOffset.MatchString(q)
}

// Using package-level regex in sqlcontext would create cycle; compile here.
var regexpSelectStar = regexp.MustCompile(`(?is)\bSELECT\s+\*\s+FROM\b`)

var regexpOffset = regexp.MustCompile(`(?is)\bOFFSET\s+\d+`)
