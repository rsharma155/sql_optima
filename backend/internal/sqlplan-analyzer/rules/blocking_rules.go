// File: internal/sqlplan_rules/blocking_rules.go
// Purpose: Blocking operator rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerBlockingOperatorRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "ExpensiveSort",
		Description: "Sort operator with high relative cost",
		Category:    "Blocking",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeBlocking,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if !strings.Contains(op.PhysicalOp, "Sort") {
				return false
			}
			totalCost := getTotalPlanCost(plan)
			if totalCost == 0 {
				return false
			}
			return (op.EstimatedTotalSubtreeCost / totalCost) > 0.5
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Expensive Sort Operation",
				fmt.Sprintf("Sort consumes %.0f%% of total plan cost. Sort is blocking - all downstream processing waits for sort to complete.", (op.EstimatedTotalSubtreeCost/getTotalPlanCost(plan))*100),
				"Optimize input data, create covering index in required order, or reduce rows to sort",
				"Blocking operator delays all downstream processing",
				models.SeverityMedium,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 10: Sort",
		Tags:        []string{"sort", "blocking", "expensive"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "SortSpillToTempDB",
		Description: "Sort operation spilled to tempdb",
		Category:    "Blocking",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeTempDB,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Sort") && op.ActualSpills > 0
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Sort Operation Spilled to tempdb",
				fmt.Sprintf("Sort exceeded memory grant and wrote %d pages to tempdb. Sort spills are extremely expensive.", op.ActualSpills),
				"Increase memory grant, reduce rows to sort, or ensure input is pre-sorted by creating index",
				"Massive disk I/O, 100x slower than memory sort",
				models.SeverityHigh,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 10: Sort Spills",
		Tags:        []string{"sort", "spill", "tempdb", "blocking"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "HashAggregate",
		Description: "Hash aggregation operation",
		Category:    "Blocking",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeBlocking,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Hash Match") && strings.Contains(op.PhysicalOp, "Aggregate")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Hash Aggregation",
				"Hash aggregate builds hash table in memory for grouping. Requires all groups to fit in memory.",
				"Consider Stream Aggregate if input is sorted by group columns. May indicate need for covering index.",
				"Memory-intensive for many groups, may spill",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 10: Aggregates",
		Tags:        []string{"hash", "aggregate", "group"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "StreamAggregate",
		Description: "Stream aggregation operation",
		Category:    "Blocking",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeBlocking,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Stream") && strings.Contains(op.PhysicalOp, "Aggregate")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Stream Aggregation",
				"Stream aggregate requires input sorted by group columns. If not pre-sorted, implicit sort is added.",
				"Ensure input is sorted by creating index on group columns",
				"May add sort if input not pre-sorted",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 10: Aggregates",
		Tags:        []string{"stream", "aggregate", "sorted"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "Concatenation",
		Description: "Concatenation (UNION ALL) operation",
		Category:    "Blocking",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeBlocking,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Concatenation")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Concatenation (UNION ALL)",
				"Concatenation combines multiple inputs without blocking. Usually efficient.",
				"No action needed - concatenation is generally efficient",
				"No significant impact",
				models.SeverityLow,
			)
		},
		Priority:    5,
		BookChapter: "Chapter 10: Concatenation",
		Tags:        []string{"concatenation", "union"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "SequenceProject",
		Description: "Sequence project operation",
		Category:    "Blocking",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeCompute,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Sequence Project")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Sequence Project",
				"Evaluates window functions and ranking. Usually not a bottleneck.",
				"No action typically needed - compute operation",
				"Minimal overhead",
				models.SeverityLow,
			)
		},
		Priority:    5,
		BookChapter: "Chapter 10: Sequence Project",
		Tags:        []string{"sequence", "window", "ranking"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "TableSpoolLazy",
		Description: "Lazy table spool operation",
		Category:    "Blocking",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeBlocking,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Table Spool") && strings.Contains(op.PhysicalOp, "Lazy")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Lazy Table Spool",
				"Materializes rows on first access, reuses for subsequent passes. Saves recomputation but uses tempdb.",
				"Review if spool can be eliminated with better index. Consider indexed view.",
				"Uses tempdb storage, added IO",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 10: Spools",
		Tags:        []string{"spool", "tempdb", "lazy"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "IndexSpool",
		Description: "Index spool operation",
		Category:    "Blocking",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeBlocking,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Index Spool")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Index Spool",
				"Creates temporary index to satisfy query. Indicates missing index or suboptimal plan.",
				"Consider creating permanent index to eliminate spool",
				"Additional write overhead, uses tempdb",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 10: Spools",
		Tags:        []string{"spool", "index", "temporary"},
	})
}

func getTotalPlanCost(plan *models.PlanAnalysis) float64 {
	if plan.QueryPlan == nil || plan.QueryPlan.RelOp == nil {
		return 0
	}
	return plan.QueryPlan.RelOp.EstimatedTotalSubtreeCost
}

