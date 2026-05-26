// File: internal/sqlplan_rules/cardinality_rules.go
// Purpose: Cardinality estimation rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerCardinalityRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "CardinalityEstimateMismatch",
		Description: "Estimated vs actual rows significantly differ",
		Category:    "Cardinality",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeEstimation,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.EstimateRows < 10 {
				return false
			}
			ratio := float64(op.ActualRows) / op.EstimateRows
			return ratio > 10 || ratio < 0.1
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			tableStr := getTableName(op)
			if tableStr == "" {
				tableStr = op.PhysicalOp
			}
			opLabel := fmt.Sprintf("[%s] %s", op.PhysicalOp, tableStr)

			var title, desc string
			if op.ActualRows == 0 {
				title = fmt.Sprintf("Cardinality Estimate %.0f, actual 0 rows on %s", op.EstimateRows, tableStr)
				desc = fmt.Sprintf("On %s: optimizer estimated %.0f rows but actual was 0 (no rows returned).", opLabel, op.EstimateRows)
			} else {
				ratio := float64(op.ActualRows) / op.EstimateRows
				if ratio < 1 {
					r := 1 / ratio
					title = fmt.Sprintf("Cardinality overestimate %.0fx on %s", r, tableStr)
					desc = fmt.Sprintf("On %s: optimizer estimated %.0f rows but actual was %d. %.0fx overestimate.", opLabel, op.EstimateRows, op.ActualRows, r)
				} else {
					title = fmt.Sprintf("Cardinality underestimate %.0fx on %s", ratio, tableStr)
					desc = fmt.Sprintf("On %s: optimizer estimated %.0f rows but actual was %d. %.0fx underestimate.", opLabel, op.EstimateRows, op.ActualRows, ratio)
				}
			}
			return makeBaseFinding(op, title, desc,
				"Update table statistics, consider trace flag 4136 (parameter sniffing) or 2312 (legacy CE), rebuild indexes",
				"Wrong memory grants, wrong join types, spills due to incorrect estimates",
				models.SeverityHigh,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 8: Cardinality Estimation",
		Tags:        []string{"cardinality", "estimate", "mismatch"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "ParameterSniffing",
		Description: "Plan optimized for specific parameter value",
		Category:    "Cardinality",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeEstimation,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return op.EstimateRebinds > 10 || op.EstimateRewinds > 10
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			recompileType := "rebinds"
			count := op.EstimateRebinds
			if op.EstimateRewinds > op.EstimateRebinds {
				recompileType = "rewinds"
				count = op.EstimateRewinds
			}
			return makeBaseFinding(op,
				"Parameter Sniffing Detected",
				fmt.Sprintf("Operator executed %d %s - same plan reused with different parameter values. Plan optimized for first parameter values.", count, recompileType),
				"Use OPTION (OPTIMIZE FOR UNKNOWN) or OPTION (RECOMPILE), or create plan guide",
				"Variable performance - fast for some parameters, slow for others",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 8: Parameter Sniffing",
		Tags:        []string{"parameter", "sniffing", "recompile"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "StaleStatistics",
		Description: "Statistics may be outdated",
		Category:    "Cardinality",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeEstimation,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 || len(plan.QueryPlan.OptimizerStatsUsage) == 0 {
				return false
			}
			for _, stats := range plan.QueryPlan.OptimizerStatsUsage {
				if stats.Modification > 1000 {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			modCount := 0
			for _, stats := range plan.QueryPlan.OptimizerStatsUsage {
				if stats.Modification > modCount {
					modCount = int(stats.Modification)
				}
			}
			return makeBaseFinding(op,
				"Stale Statistics Detected",
				fmt.Sprintf("Table statistics have been modified %d times since last update. Outdated statistics lead to poor estimates.", modCount),
				"Run UPDATE STATISTICS with FULL SCAN on affected tables",
				"Poor plan selection based on outdated data distribution",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 8: Statistics",
		Tags:        []string{"statistics", "stale", "outdated"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "ComplexPredicate",
		Description: "Complex predicate may confuse optimizer",
		Category:    "Cardinality",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypePredicate,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.Predicate == nil {
				return false
			}
			predicate := op.Predicate.ScalarOperator
			return len(predicate) > 200 && strings.Count(predicate, "AND") > 3
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Complex Predicate May Confuse Optimizer",
				"Predicate has many conditions (4+ ANDs). Optimizer may underestimate or fail to use indexes properly.",
				"Consider simplifying predicate, using OPTION (RECOMPILE), or breaking into multiple queries",
				"Potential suboptimal plan due to complex filter evaluation",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 7: Predicates and Filters",
		Tags:        []string{"predicate", "complex", "optimizer"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "RowGoalIssue",
		Description: "Row goal affecting plan selection",
		Category:    "Cardinality",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeEstimation,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return op.TopRowCount != "" && op.EstimateRows > 10000
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Row Goal Affecting Plan Selection",
				"TOP operator creates row goal, optimizer chooses fast partial scan instead of optimal full scan. May change if more rows needed.",
				"Review TOP N value. If variable, consider OPTION (OPTIMIZE FOR UNKNOWN) or adjust row goal",
				"Plan may be suboptimal for larger result sets",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 8: Row Goals",
		Tags:        []string{"rowgoal", "top", "optimization"},
	})
}

