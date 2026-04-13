package parser

import (
	"regexp"
	"strings"

	"github.com/rsharma155/sql_optima/internal/explain/types"
)

func ParseNodeLine(line string) *types.PlanNode {
	re := regexp.MustCompile(`^(\w+(?:\s+\w+)*)\s+on\s+(\w+)`)
	m := re.FindStringSubmatch(line)

	var node *types.PlanNode
	if m != nil {
		nodeType := strings.TrimSpace(m[1])
		if idx := strings.Index(nodeType, " using"); idx > 0 {
			nodeType = nodeType[:idx]
		}
		node = &types.PlanNode{
			NodeType:     nodeType,
			RelationName: strings.TrimSpace(m[2]),
		}
	} else {
		re3 := regexp.MustCompile(`^(\w+(?:\s+\w+)*)`)
		m3 := re3.FindStringSubmatch(line)
		if m3 == nil {
			return nil
		}
		nodeType := strings.TrimSpace(m3[1])
		if idx := strings.Index(nodeType, " using"); idx > 0 {
			nodeType = nodeType[:idx]
		}
		node = &types.PlanNode{
			NodeType: nodeType,
		}
	}

	re = regexp.MustCompile(`using\s+(\w+)`)
	m = re.FindStringSubmatch(line)
	if m != nil && node.IndexName == "" {
		node.IndexName = m[1]
	}

	re = regexp.MustCompile(`\(([^)]+)\)`)
	matches := re.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		info := match[1]
		ParseCostInfo(info, node)
		ParseActualInfo(info, node)
	}

	return node
}

func ExtractNodeType(line string) string {
	re := regexp.MustCompile(`^(\w+(?:\s+\w+)*)`)
	m := re.FindStringSubmatch(line)
	if m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}
