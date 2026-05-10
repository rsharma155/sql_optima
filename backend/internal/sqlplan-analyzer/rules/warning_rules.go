// File: internal/sqlplan_rules/warning_rules.go
// Purpose: Plan warning rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerWarningRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningSpillToTempDB",
		Description: "Plan warning: operation spilled to tempdb",
		Category:    "Warnings",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeSpillToTempDB {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			var spillInfo string
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeSpillToTempDB {
					spillInfo = w.Message
					break
				}
			}
			return makeBaseFinding(op,
				"Plan Warning: Spill to tempdb",
				spillInfo,
				"Increase memory grant, optimize query to reduce memory needs, or add index to pre-sort data",
				"Massive disk I/O, query may timeout",
				models.SeverityHigh,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "spill", "tempdb"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningNoJoinPredicate",
		Description: "Plan warning: no join predicate",
		Category:    "Warnings",
		Severity:    models.SeverityCritical,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeNoJoinPredicate {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: No Join Predicate (Cartesian Product)",
				"Query joins tables without ON clause. This creates Cartesian product of all rows.",
				"Add proper ON clause or WHERE condition to join tables",
				"Exponential row count, potential timeout, tempdb overflow",
				models.SeverityCritical,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "no-join", "cartesian"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningTypeConversion",
		Description: "Plan warning: type conversion in predicate",
		Category:    "Warnings",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeTypeConversion {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: Implicit Type Conversion",
				"Query contains implicit type conversion that prevents index usage.",
				"Cast parameter explicitly to match column type",
				"Full scan instead of seek, high IO",
				models.SeverityHigh,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "conversion", "type"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningCardinalityEstimate",
		Description: "Plan warning: cardinality estimate issue",
		Category:    "Warnings",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeCardinalityEst {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: Cardinality Estimate Issue",
				"Optimizer detected potential cardinality estimate problem. Estimates may not match actual data distribution.",
				"Update statistics, consider trace flags for CE, review data skew",
				"Suboptimal plan selection",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "cardinality", "estimate"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningNoStatistics",
		Description: "Plan warning: no statistics available",
		Category:    "Warnings",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeNoStatistics {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: No Statistics",
				"Statistics not available for object. Optimizer using default estimates.",
				"Create statistics manually or run UPDATE STATISTICS",
				"Poor estimates, potentially wrong plan",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "statistics", "missing"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningUnmatchedIndex",
		Description: "Plan warning: unmatched index in hint",
		Category:    "Warnings",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeUnmatchedIndex {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: Unmatched Index",
				"Index specified in hint does not exist or cannot be used.",
				"Verify index name, check if index was renamed or dropped",
				"Hint ignored, suboptimal plan",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "index", "unmatched"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningFunctionOnIndex",
		Description: "Plan warning: function on index column",
		Category:    "Warnings",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeFunctionOnIndex {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: Function on Index Column",
				"Column used with function in predicate prevents index usage.",
				"Rewrite predicate to isolate column, or create computed column index",
				"Index not used, full scan required",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "function", "index"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningColumnsNotCovered",
		Description: "Plan warning: columns not covered by index",
		Category:    "Warnings",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeColumnsNotCovered {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: Columns Not Covered",
				"Index does not include all required columns, causing bookmark lookup.",
				"Add missing columns to INCLUDE clause of index",
				"Bookmark lookup, high IO",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "covering", "index"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningWaitFor",
		Description: "Plan warning: wait for resource",
		Category:    "Warnings",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeWaitFor {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: Wait For",
				"Query waiting for resource (e.g., WAITFOR DELAY). This is intentional for polling scenarios.",
				"Review if wait is necessary, consider eliminating if not needed",
				"Intentional delay or blocking",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "wait", "blocking"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "PlanWarningTrivialPlan",
		Description: "Plan warning: trivial plan selected",
		Category:    "Warnings",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeWarning,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.ID != 1 {
				return false
			}
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeTrivialPlan {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Plan Warning: Trivial Plan",
				"Optimizer selected trivial plan with limited optimization. May miss optimization opportunities.",
				"Review query for missing indexes, constraints, or consider OPTION (NORECOMPILE)",
				"Limited optimization scope",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 11: Warnings",
		Tags:        []string{"warning", "trivial", "plan"},
	})
}

