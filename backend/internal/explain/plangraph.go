package explain

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

// PlanGraphNode is a node for tree/flow visualizations (client renders Mermaid or HTML).
type PlanGraphNode struct {
	ID             string  `json:"id"`
	ParentID       string  `json:"parent_id,omitempty"`
	NodeType       string  `json:"node_type"`
	RelationName   string  `json:"relation_name,omitempty"`
	Alias          string  `json:"alias,omitempty"`
	IndexName      string  `json:"index_name,omitempty"`
	TotalCost      float64 `json:"total_cost"`
	PlanRows       int     `json:"plan_rows"`
	ActualRows     int     `json:"actual_rows,omitempty"`
	ActualTotalMs  float64 `json:"actual_total_time_ms,omitempty"`
	ExclusiveMs    float64 `json:"exclusive_time_ms,omitempty"`
	Filter         string  `json:"filter,omitempty"`
	IndexCond      string  `json:"index_cond,omitempty"`
	PlannerNodeID  int     `json:"planner_node_id,omitempty"`
}

// PlanGraph is a flat node list + edges for DAG visualization.
type PlanGraph struct {
	Nodes []PlanGraphNode `json:"nodes"`
	Edges []struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"edges"`
}

var graphSeq int

func nextGraphID() string {
	graphSeq++
	return fmt.Sprintf("g%d", graphSeq)
}

// BuildPlanGraph walks the plan tree and returns nodes/edges for graphical renderers.
func BuildPlanGraph(root types.PlanNode) PlanGraph {
	graphSeq = 0
	var out PlanGraph
	walkPlanGraph(&root, "", &out)
	return out
}

func walkPlanGraph(n *types.PlanNode, parentGraphID string, out *PlanGraph) string {
	id := nextGraphID()

	gn := PlanGraphNode{
		ID:            id,
		ParentID:      parentGraphID,
		NodeType:      n.NodeType,
		RelationName:  n.RelationName,
		Alias:         n.Alias,
		IndexName:     n.IndexName,
		TotalCost:     n.TotalCost,
		PlanRows:      n.PlanRows,
		ActualRows:    n.ActualRows,
		ActualTotalMs: n.ActualTotalTime,
		ExclusiveMs:   n.ExclusiveTime,
		Filter:        truncateStr(n.Filter, 120),
		IndexCond:     truncateStr(n.IndexCond, 120),
		PlannerNodeID: n.ID,
	}
	out.Nodes = append(out.Nodes, gn)
	if parentGraphID != "" {
		out.Edges = append(out.Edges, struct {
			From string `json:"from"`
			To   string `json:"to"`
		}{From: parentGraphID, To: id})
	}
	for i := range n.Plans {
		walkPlanGraph(&n.Plans[i], id, out)
	}
	return id
}

func truncateStr(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// MermaidFlowchart returns a Mermaid diagram source (escape handled on client).
func MermaidFlowchart(g PlanGraph) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, n := range g.Nodes {
		lbl := n.NodeType
		if n.RelationName != "" {
			lbl += " · " + n.RelationName
		}
		if n.ActualTotalMs > 0 {
			lbl += fmt.Sprintf("\\n%.2f ms", n.ActualTotalMs)
		} else if n.TotalCost > 0 {
			lbl += fmt.Sprintf("\\ncost %.0f", n.TotalCost)
		}
		lbl = strings.ReplaceAll(lbl, `"`, "'")
		b.WriteString(fmt.Sprintf(`  %s["%s"]`+"\n", n.ID, lbl))
	}
	for _, e := range g.Edges {
		b.WriteString(fmt.Sprintf("  %s --> %s\n", e.From, e.To))
	}
	return b.String()
}
