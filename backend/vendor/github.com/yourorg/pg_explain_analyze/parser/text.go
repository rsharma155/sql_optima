package parser

import (
	"encoding/json"
	"strings"

	"github.com/yourorg/pg_explain_analyze/types"
)

func ParseText(input string) (*types.Plan, error) {
	lines := strings.Split(input, "\n")

	plan := &types.Plan{
		Settings: make(map[string]string),
	}

	type nodeEntry struct {
		node   *types.PlanNode
		indent int
	}
	var stack []nodeEntry

	var currentNode *types.PlanNode

	processProperty := func(line string) {
		if currentNode != nil {
			ApplyProperty(line, currentNode)
		}
	}

	processNodeLine := func(line string, indent int, isArrow bool) *types.PlanNode {
		if isArrow {
			line = strings.TrimPrefix(line, "->")
			line = strings.TrimSpace(line)
		}

		if !strings.Contains(line, "(") && !isArrow {
			return nil
		}

		node := ParseNodeLine(line)
		if node == nil && isArrow {
			node = &types.PlanNode{NodeType: ExtractNodeType(line)}
		}
		if node == nil {
			return nil
		}

		if len(stack) == 0 {
			plan.Plan = *node
			currentNode = &plan.Plan
			stack = append(stack, nodeEntry{node: &plan.Plan, indent: indent})
			return &plan.Plan
		}

		newIndent := indent
		if isArrow {
			newIndent = indent + 4
		}

		for len(stack) > 0 && stack[len(stack)-1].indent >= newIndent {
			stack = stack[:len(stack)-1]
		}

		if len(stack) > 0 {
			parent := stack[len(stack)-1].node
			parent.Plans = append(parent.Plans, *node)
			currentNode = &parent.Plans[len(parent.Plans)-1]
			stack = append(stack, nodeEntry{node: currentNode, indent: newIndent})
		}

		return node
	}

	for i, rawLine := range lines {
		line := strings.TrimSpace(rawLine)

		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "##") {
			continue
		}

		if strings.HasPrefix(line, "=>") {
			query := strings.TrimSpace(strings.TrimPrefix(line, "=>"))
			query = strings.TrimPrefix(query, "EXPLAIN")
			plan.Query = strings.TrimSpace(query)
			continue
		}

		if strings.HasPrefix(line, "QUERY PLAN") {
			continue
		}

		if strings.HasPrefix(line, "──") || strings.HasPrefix(line, "----") {
			continue
		}

		if strings.HasPrefix(line, "(") {
			continue
		}

		if strings.HasPrefix(line, "Planning") || strings.HasPrefix(line, "Planning Time:") {
			plan.PlanningTime = ParsePlanningTime(line)
			continue
		}
		if strings.HasPrefix(line, "Execution") || strings.HasPrefix(line, "Execution Time:") {
			plan.ExecutionTime = ParseExecutionTime(line)
			continue
		}

		if strings.HasPrefix(line, "JIT:") || strings.HasPrefix(line, "Functions:") {
			lines := lines[i:]
			ParseJITInfo(lines, plan)
			continue
		}

		indent := countIndent(rawLine)
		hasArrow := strings.HasPrefix(line, "->")
		isProp := IsPropertyLine(line)

		if isProp {
			processProperty(line)
			continue
		}

		if hasArrow {
			processNodeLine(line, indent, true)
			continue
		}

		if strings.Contains(line, "(") && strings.Contains(line, "cost=") {
			processNodeLine(line, indent, false)
			continue
		}

		if strings.Contains(line, "actual time=") {
			if currentNode != nil {
				ParseActualLine(line, currentNode)
			}
			continue
		}

		if strings.HasPrefix(line, "(") && strings.Contains(line, "actual time=") {
			if currentNode != nil {
				ParseActualLine(line, currentNode)
			}
			continue
		}

		if strings.HasPrefix(line, "(") {
			continue
		}

		if strings.Contains(line, "(") && strings.Contains(line, "width=") && !hasArrow {
			processNodeLine(line, indent, false)
			continue
		}

		if hasArrow || strings.Contains(line, "(") {
			processNodeLine(line, indent, hasArrow)
		}
	}

	if plan.Plan.NodeType == "" && len(plan.Plan.Plans) > 0 {
		firstPlan := plan.Plan.Plans[0]
		plan.Plan = firstPlan
		plan.Plan.Plans = plan.Plan.Plans[1:]
		if plan.Plan.Parent != nil {
			plan.Plan.Parent = nil
		}
	}

	return plan, nil
}

func findNodeByPtr(root *types.PlanNode, target *types.PlanNode) *types.PlanNode {
	if root == target {
		return root
	}
	for i := range root.Plans {
		found := findNodeByPtr(&root.Plans[i], target)
		if found != nil {
			return found
		}
	}
	return nil
}

func ParseJSON(input string) (*types.Plan, error) {
	var plan types.Plan
	err := json.Unmarshal([]byte(input), &plan)
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func getChildTypes(node *types.PlanNode) []string {
	var types []string
	for _, child := range node.Plans {
		types = append(types, child.NodeType)
	}
	return types
}
