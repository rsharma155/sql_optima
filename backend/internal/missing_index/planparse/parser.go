package planparse

import (
	"context"
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/missing_index/logging"
	"github.com/rsharma155/sql_optima/internal/missing_index/types"
)

func Parse(ctx context.Context, planJSON map[string]any) (*types.PlanAnalysis, error) {
	if planJSON == nil {
		return nil, nil
	}

	// Handle array format: [{"Plan": {...}}]
	if plans, ok := planJSON["Plan"].([]any); ok && len(plans) > 0 {
		if firstPlan, ok := plans[0].(map[string]any); ok {
			planJSON = map[string]any{"Plan": firstPlan}
		}
	}

	logging.Info(ctx, "Plan JSON structure", map[string]any{
		"has_root": planJSON["Plan"] != nil,
		"keys":     len(planJSON),
	})

	analysis := &types.PlanAnalysis{
		RootNode:      parseNode(planJSON),
		TargetTables:  []types.TableRef{},
		Opportunities: []types.TableOpportunity{},
	}

	if analysis.RootNode == nil {
		logging.Warn(ctx, "Root node is nil after parsing", map[string]any{
			"keys_in_json": len(planJSON),
		})
		return analysis, nil
	}

	analysis.TotalCost = analysis.RootNode.TotalCost
	analysis.StartupCost = analysis.RootNode.StartupCost
	analysis.PlanRows = analysis.RootNode.PlanRows
	extractTargetTables(analysis.RootNode, &analysis.TargetTables)
	identifyOpportunities(analysis.RootNode, &analysis.Opportunities)

	return analysis, nil
}

func parseNode(nodeMap map[string]any) *types.PlanNode {
	if nodeMap == nil {
		return nil
	}

	node := &types.PlanNode{}

	if v, ok := nodeMap["Node Type"].(string); ok {
		node.NodeType = v
	}

	if v, ok := nodeMap["Relation Name"].(string); ok {
		node.RelationName = &v
	}

	if v, ok := nodeMap["Alias"].(string); ok {
		node.Alias = &v
	}

	if v, ok := nodeMap["Schema"].(string); ok {
		node.Schema = &v
	}

	if v, ok := nodeMap["Index Name"].(string); ok {
		node.IndexName = &v
	}

	if v, ok := nodeMap["Filter"].(string); ok {
		node.Filter = &v
	}

	// Check if this is a Plan wrapper
	if node.NodeType == "" {
		if plan, ok := nodeMap["Plan"].(map[string]any); ok {
			return parseNode(plan)
		}
	}

	if arr, ok := nodeMap["Index Cond"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				node.IndexCond = append(node.IndexCond, s)
			}
		}
	}

	if arr, ok := nodeMap["Hash Cond"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				node.HashCond = append(node.HashCond, s)
			}
		}
	}

	if arr, ok := nodeMap["Merge Cond"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				node.MergeCond = append(node.MergeCond, s)
			}
		}
	}

	if arr, ok := nodeMap["Sort Key"].([]any); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				node.SortKey = append(node.SortKey, s)
			}
		}
	}

	if v, ok := nodeMap["Startup Cost"].(float64); ok {
		node.StartupCost = v
	}

	if v, ok := nodeMap["Total Cost"].(float64); ok {
		node.TotalCost = v
	}

	if v, ok := nodeMap["Plan Rows"].(float64); ok {
		node.PlanRows = int64(v)
	}

	if v, ok := nodeMap["Plan Width"].(float64); ok {
		node.PlanWidth = int64(v)
	}

	if arr, ok := nodeMap["Plans"].([]any); ok {
		for _, child := range arr {
			if childMap, ok := child.(map[string]any); ok {
				if childNode := parseNode(childMap); childNode != nil {
					node.Children = append(node.Children, childNode)
				}
			}
		}
	}

	return node
}

func extractTargetTables(node *types.PlanNode, tables *[]types.TableRef) {
	if node == nil || node.NodeType == "" {
		return
	}

	interestingTypes := map[string]bool{
		"Seq Scan":         true,
		"Index Scan":       true,
		"Index Only Scan":  true,
		"Bitmap Heap Scan": true,
		"Tid Scan":         true,
		"Subquery Scan":    true,
	}

	if interestingTypes[node.NodeType] && node.RelationName != nil {
		schema := "public"
		if node.Schema != nil {
			schema = *node.Schema
		}
		*tables = append(*tables, types.TableRef{
			Schema: schema,
			Name:   *node.RelationName,
		})
	}

	for _, child := range node.Children {
		extractTargetTables(child, tables)
	}
}

func identifyOpportunities(node *types.PlanNode, opportunities *[]types.TableOpportunity) {
	if node == nil {
		return
	}

	expensiveTypes := map[string]bool{
		"Seq Scan":         true,
		"Bitmap Heap Scan": true,
		"Sort":             true,
		"Nested Loop":      true,
		"Hash Join":        true,
		"Merge Join":       true,
	}

	if expensiveTypes[node.NodeType] && node.RelationName != nil {
		schema := "public"
		if node.Schema != nil {
			schema = *node.Schema
		}

		opp := types.TableOpportunity{
			Table:           types.TableRef{Schema: schema, Name: *node.RelationName},
			ScanType:        node.NodeType,
			CurrentCost:     node.TotalCost,
			EstimatedRows:   node.PlanRows,
			HasSortPressure: node.NodeType == "Sort" || len(node.SortKey) > 0,
		}

		if node.Filter != nil {
			opp.FilterColumns = extractColumnsFromFilter(*node.Filter)
		}

		opp.OrderByColumns = node.SortKey

		if len(node.HashCond) > 0 {
			opp.JoinColumns = extractColumnsFromHashCond(node.HashCond)
		}
		if len(node.MergeCond) > 0 {
			opp.JoinColumns = append(opp.JoinColumns, extractColumnsFromMergeCond(node.MergeCond)...)
		}

		*opportunities = append(*opportunities, opp)
	}

	for _, child := range node.Children {
		identifyOpportunities(child, opportunities)
	}
}

func extractColumnsFromFilter(filter string) []string {
	if strings.TrimSpace(filter) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var cols []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.EqualFold(s, "true") || strings.EqualFold(s, "false") {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		cols = append(cols, s)
	}
	// (alias.col op ... or (col op ... ; LIKE/IS/IN; cast patterns
	re := regexp.MustCompile(`(?i)\(\s*(?:(\w+)\.)?(\w+)\s*(?:=|<>|!=|<|>|<=|>=|~~|!~~|\bLIKE\b|\bIS\b|\bIN\b)`)
	for _, m := range re.FindAllStringSubmatch(filter, -1) {
		if len(m) >= 3 && m[2] != "" {
			add(m[2])
		}
	}
	return cols
}

func extractColumnsFromHashCond(conds []string) []string {
	var cols []string
	for _, cond := range conds {
		re := regexp.MustCompile(`(\w+)\s*=\s*(\w+)`)
		matches := re.FindAllStringSubmatch(cond, -1)
		for _, m := range matches {
			if len(m) > 1 {
				cols = append(cols, m[1])
			}
			if len(m) > 2 {
				cols = append(cols, m[2])
			}
		}
	}
	return cols
}

func extractColumnsFromMergeCond(conds []string) []string {
	return extractColumnsFromHashCond(conds)
}
