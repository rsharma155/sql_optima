package explain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

// SQLContextMeta documents limitations of heuristic SQL linking.
const SQLContextDisclaimer = "PostgreSQL does not expose exact character ranges from plan nodes to SQL text. " +
	"We match relation/alias names and filters heuristically—verify against your query."

// FindingSQLContext augments a finding when the user supplied SQL text.
type FindingSQLContext struct {
	FindingType     string   `json:"finding_type"`
	Severity        string   `json:"severity"`
	Message         string   `json:"message"`
	NodeID          int      `json:"node_id,omitempty"`
	PlannerNodeType string   `json:"planner_node_type,omitempty"`
	RelationName    string   `json:"relation_name,omitempty"`
	// Line numbers are 1-based; best-effort match to a line containing the relation.
	LineStart       int    `json:"line_start,omitempty"`
	LineEnd         int    `json:"line_end,omitempty"`
	SQLExcerpt      string `json:"sql_excerpt,omitempty"`
	MatchConfidence string `json:"match_confidence,omitempty"` // high | medium | low
	// Concrete DDL suggestion when columns could be inferred from Filter / Index Cond.
	SuggestedIndexSQL string `json:"suggested_index_sql,omitempty"`
	RewriteHint       string `json:"rewrite_hint,omitempty"`
}

// SQLContextBundle is attached to analyze/optimize responses when query is non-empty.
type SQLContextBundle struct {
	Disclaimer        string                 `json:"disclaimer"`
	Findings          []FindingSQLContext    `json:"findings"`
	HeuristicInsights []PlanHeuristicInsight `json:"heuristic_insights,omitempty"`
}

// nodeByPlannerID maps flattened planner-assigned node IDs after analysis.
func nodeByPlannerID(nodes []types.PlanNode) map[int]*types.PlanNode {
	m := make(map[int]*types.PlanNode)
	for i := range nodes {
		n := &nodes[i]
		if n.ID > 0 {
			m[n.ID] = n
		}
	}
	return m
}

// AugmentFindingsWithSQL builds per-finding context from the user query and plan nodes.
func AugmentFindingsWithSQL(query string, findings []types.Finding, flatNodes []types.PlanNode) SQLContextBundle {
	q := strings.TrimSpace(query)
	bundle := SQLContextBundle{
		Disclaimer: SQLContextDisclaimer,
		Findings:   nil,
	}
	if q == "" || len(findings) == 0 {
		return bundle
	}
	byID := nodeByPlannerID(flatNodes)
	for _, f := range findings {
		ctx := FindingSQLContext{
			FindingType: f.Type,
			Severity:    string(f.Severity),
			Message:     f.Message,
			NodeID:      f.NodeID,
			RelationName: func() string {
				if f.RelationName != "" {
					return f.RelationName
				}
				if n, ok := byID[f.NodeID]; ok {
					return n.RelationName
				}
				return ""
			}(),
		}
		var n *types.PlanNode
		if f.NodeID > 0 {
			n = byID[f.NodeID]
		}
		if n != nil {
			ctx.PlannerNodeType = n.NodeType
			if ctx.RelationName == "" {
				ctx.RelationName = n.RelationName
			}
			ctx.SuggestedIndexSQL = suggestIndexDDL(n)
			ctx.RewriteHint = buildRewriteHint(n, f)
		} else {
			// No node id: still try relation from finding
			ctx.SuggestedIndexSQL = suggestIndexDDL(&types.PlanNode{
				RelationName: f.RelationName,
				NodeType:     f.NodeType,
			})
		}

		lineStart, lineEnd, excerpt, conf := locateRelationInQuery(q, ctx.RelationName, aliasForNode(n))
		if excerpt != "" {
			ctx.LineStart, ctx.LineEnd = lineStart, lineEnd
			ctx.SQLExcerpt = excerpt
			ctx.MatchConfidence = conf
		} else if n != nil && n.Filter != "" {
			// Try to find a column from the filter appearing in SQL
			for _, col := range extractIndexColumns(n.Filter + " " + n.IndexCond) {
				if ls, le, ex, c := locateColumnInQuery(q, col); ex != "" {
					ctx.LineStart, ctx.LineEnd = ls, le
					ctx.SQLExcerpt = ex
					ctx.MatchConfidence = c
					break
				}
			}
		}

		bundle.Findings = append(bundle.Findings, ctx)
	}
	return bundle
}

func aliasForNode(n *types.PlanNode) string {
	if n == nil {
		return ""
	}
	return n.Alias
}

func buildRewriteHint(n *types.PlanNode, f types.Finding) string {
	if n == nil {
		return ""
	}
	var parts []string
	switch {
	case strings.Contains(n.NodeType, "Seq Scan"):
		parts = append(parts, "Sequential scan means the planner did not use an index for this relation.")
		if n.Filter != "" {
			parts = append(parts, "Predicate: "+truncateStr(n.Filter, 160))
		}
		parts = append(parts, "Consider an index that matches the filter/WHERE predicates, or reduce selected rows.")
	case strings.Contains(n.NodeType, "Nested Loop"):
		parts = append(parts, "Nested loop can be expensive when the inner side runs many times.")
		parts = append(parts, "Check join order and row counts; hash join may help if work_mem allows.")
	case strings.Contains(f.Type, "RowEstimation"):
		parts = append(parts, "Stale statistics can skew plans; run ANALYZE on involved tables.")
	default:
		if f.Suggestion != "" {
			return f.Suggestion
		}
	}
	return strings.Join(parts, " ")
}

// suggestIndexDDL builds a concrete CREATE INDEX when columns are inferable.
func suggestIndexDDL(n *types.PlanNode) string {
	if n == nil || n.RelationName == "" {
		return ""
	}
	rel := safeQualifiedIdent(n.RelationName)
	if rel == "" {
		return ""
	}
	cols := extractIndexColumns(strings.Join([]string{
		n.IndexCond,
		n.Filter,
		n.JoinFilter,
		n.HashCond,
		n.MergeCond,
	}, " "))
	if len(cols) == 0 {
		msg := truncateStr(strings.TrimSpace(n.Filter), 200)
		if msg == "" {
			msg = truncateStr(strings.TrimSpace(n.IndexCond), 200)
		}
		if msg == "" {
			msg = "(no filter/index condition captured)"
		}
		return fmt.Sprintf("-- Review filter/join keys on %s and add an index on those columns.\n-- Condition: %s", rel, msg)
	}
	suffix := strings.Join(cols, "_")
	if len(suffix) > 48 {
		suffix = suffix[:48]
	}
	colList := strings.Join(cols, ", ")
	return fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_%s_%s ON %s (%s);",
		rel, suffix, rel, colList)
}

var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func safeIdent(s string) string {
	s = strings.TrimSpace(s)
	if identRe.MatchString(s) {
		return s
	}
	return ""
}

func safeQualifiedIdent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 2 {
		return ""
	}
	for i := range parts {
		if safeIdent(parts[i]) == "" {
			return ""
		}
	}
	return strings.Join(parts, ".")
}

// extractIndexColumns pulls left-hand column names from simple PG filter patterns.
func extractIndexColumns(expr string) []string {
	if expr == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	// Try to catch:
	// - (col = ...)
	// - (alias.col = ...)
	// - ((col)::text = ...)
	// - (("col") = ...) [best-effort: we ignore quoted identifiers]
	re := regexp.MustCompile(`\(\s*(?:\(+\s*)?(?:(?:[a-zA-Z_][a-zA-Z0-9_]*\.)?([a-zA-Z_][a-zA-Z0-9_]*))\s*(?:::?[a-zA-Z_][a-zA-Z0-9_]*\b)?\s*(?:=|<>|>|<|>=|<=)`)
	for _, m := range re.FindAllStringSubmatch(expr, -1) {
		c := m[1]
		if isSQLKeyword(c) {
			continue
		}
		if _, ok := seen[c]; !ok {
			seen[c] = struct{}{}
			out = append(out, c)
		}
	}
	return out
}

func isSQLKeyword(s string) bool {
	kw := strings.ToUpper(s)
	switch kw {
	case "AND", "OR", "NOT", "NULL", "TRUE", "FALSE", "CASE", "WHEN", "THEN", "ELSE", "END", "IS", "IN", "LIKE", "BETWEEN":
		return true
	default:
		return false
	}
}

func locateRelationInQuery(query, relation, alias string) (lineStart, lineEnd int, excerpt, confidence string) {
	if relation == "" {
		return 0, 0, "", ""
	}
	lines := strings.Split(query, "\n")
	relRe := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(relation) + `\b`)
	var aliasRe *regexp.Regexp
	if alias != "" && !strings.EqualFold(alias, relation) {
		aliasRe = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(alias) + `\b`)
	}

	tryMatch := func(i int, line string, conf string) (int, int, string, string) {
		ex := strings.TrimSpace(line)
		if len(ex) > 220 {
			ex = ex[:217] + "…"
		}
		return i + 1, i + 1, ex, conf
	}

	for i, line := range lines {
		if !relRe.MatchString(line) && (aliasRe == nil || !aliasRe.MatchString(line)) {
			continue
		}
		up := strings.ToUpper(strings.TrimSpace(line))
		if strings.Contains(up, "FROM") || strings.Contains(up, "JOIN") || strings.Contains(up, "UPDATE") ||
			strings.Contains(up, "INTO") || strings.Contains(up, "DELETE FROM") {
			ls, le, ex, c := tryMatch(i, line, "high")
			return ls, le, ex, c
		}
	}
	for i, line := range lines {
		if relRe.MatchString(line) || (aliasRe != nil && aliasRe.MatchString(line)) {
			return tryMatch(i, line, "medium")
		}
	}
	return 0, 0, "", ""
}

func locateColumnInQuery(query, column string) (lineStart, lineEnd int, excerpt, confidence string) {
	if column == "" {
		return 0, 0, "", ""
	}
	colRe := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(column) + `\b`)
	lines := strings.Split(query, "\n")
	for i, line := range lines {
		if colRe.MatchString(line) {
			ex := strings.TrimSpace(line)
			if len(ex) > 220 {
				ex = ex[:217] + "…"
			}
			return i + 1, i + 1, ex, "medium"
		}
	}
	return 0, 0, "", ""
}
