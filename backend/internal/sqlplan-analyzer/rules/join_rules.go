// File: internal/sqlplan_rules/join_rules.go
// Purpose: Join strategy rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerJoinRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "HashMatchJoinDetection",
		Description: "Hash match join operation",
		Category:    "Joins",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeJoin,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Hash Match") && strings.Contains(op.PhysicalOp, "Join")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Hash Match Join",
				"Hash join builds hash table in memory from smaller input, then probes with larger input. Good for large, unsorted datasets.",
				"Ensure equality join predicate exists. Consider merge join if inputs are sorted on join columns.",
				"Memory-intensive, may spill for very large datasets",
				models.SeverityLow,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 4: Hash Match",
		Tags:        []string{"hash", "join"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "NestedLoopsJoinDetection",
		Description: "Nested loops join operation",
		Category:    "Joins",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeJoin,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Nested Loops") && strings.Contains(op.PhysicalOp, "Join")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Nested Loops Join",
				"Nested loops executes outer input once and inner input for each outer row. Efficient when inner side is small with index support.",
				"Ensure inner side has index on join column. Good for small result sets.",
				"Can be expensive if inner side is large without index",
				models.SeverityLow,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 4: Nested Loops",
		Tags:        []string{"nestedloops", "join"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "MergeJoinDetection",
		Description: "Merge join operation",
		Category:    "Joins",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeJoin,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Merge") && strings.Contains(op.PhysicalOp, "Join")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Merge Join",
				"Merge join requires both inputs sorted on join columns. Combines sorted inputs like zipper. Very efficient when inputs are sorted.",
				"Verify inputs are sorted on join columns. Consider explicit sort if not, but verify sort cost is worthwhile.",
				"Requires sorted inputs, may add sort if not pre-sorted",
				models.SeverityLow,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 4: Merge Join",
		Tags:        []string{"merge", "join", "sorted"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "AdaptiveNestedLoops",
		Description: "Adaptive join using nested loops fallback",
		Category:    "Joins",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeJoin,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.NestedLoops == nil {
				return false
			}
			return op.NestedLoops.OuterIsAdaptive
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Adaptive Nested Loops Join",
				"SQL Server dynamically chooses between nested loops and hash join based on first rows. Optimizer unsure of optimal strategy.",
				"Monitor which path is taken. Consider hinting join type if consistently one path chosen.",
				"Runtime decision may vary based on data",
				models.SeverityMedium,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 4: Adaptive Joins",
		Tags:        []string{"adaptive", "nestedloops"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "ComplexHashProbeResidual",
		Description: "Hash join with probe residual predicate",
		Category:    "Joins",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeJoin,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if !strings.Contains(op.PhysicalOp, "Hash Match") || op.Hash == nil {
				return false
			}
			return op.Hash.ProbeResidual != ""
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Hash Join with Probe Residual",
				"Hash join performs additional filter after equality match. This means not all hash-probed rows qualify.",
				"Investigate residual predicate. May indicate join condition issue or additional filter needed.",
				"Additional CPU for residual check",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 4: Hash Match",
		Tags:        []string{"hash", "residual", "probe"},
	})
}

