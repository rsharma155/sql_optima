// File: internal/sqlplan_rules/predicate_rules.go
// Purpose: Predicate and SARGability rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerPredicateRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "ImplicitConversion",
		Description: "Implicit type conversion in predicate",
		Category:    "Predicate",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypePredicate,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.Predicate == nil {
				return false
			}
			return strings.Contains(op.Predicate.ScalarOperator, "CONVERT_IMPLICIT")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Implicit Type Conversion in Predicate",
				"CONVERT_IMPLICIT detected - SQL Server converts parameter to match column type. This prevents index usage and causes full scan.",
				"Explicitly cast parameter to match column type in your application or T-SQL",
				"Full table/index scan instead of seek, significant IO overhead",
				models.SeverityHigh,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 7: Implicit Conversions",
		Tags:        []string{"conversion", "implicit", "type", "seek"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "NonSargableLeadingWildcard",
		Description: "LIKE pattern with leading wildcard",
		Category:    "Predicate",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypePredicate,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.Predicate == nil {
				return false
			}
			predicate := op.Predicate.ScalarOperator
			return strings.Contains(predicate, "LIKE") && strings.Contains(predicate, "'%")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Non-SARGable LIKE with Leading Wildcard",
				"LIKE pattern starting with '%' cannot use index seek. Database must evaluate every row.",
				"Avoid leading wildcards. Consider full-text search, indexed computed columns, or reverse search pattern.",
				"Full index/table scan required",
				models.SeverityMedium,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 7: SARGability",
		Tags:        []string{"like", "sargable", "wildcard", "pattern"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "FunctionOnColumn",
		Description: "Function applied to indexed column",
		Category:    "Predicate",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypePredicate,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.Predicate == nil {
				return false
			}
			functions := []string{"CONVERT", "CAST", "YEAR(", "MONTH(", "DAY(", "DATEADD", "DATEDIFF", "LEN(", "DATALENGTH(", "SUBSTRING", "LEFT(", "RIGHT("}
			predicate := op.Predicate.ScalarOperator
			for _, fn := range functions {
				if strings.Contains(predicate, fn) {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Function Applied to Column Prevents Index Usage",
				"Column wrapped in function cannot use index seek. SQL Server must apply function to every row.",
				"Rewrite to isolate column on one side of comparison, or create computed column index",
				"Full scan required regardless of available indexes",
				models.SeverityMedium,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 7: Functions on Columns",
		Tags:        []string{"function", "column", "sargable"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "NoJoinPredicate",
		Description: "Missing join predicate - Cartesian product",
		Category:    "Predicate",
		Severity:    models.SeverityCritical,
		RuleType:    RuleTypePredicate,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			for _, w := range plan.Warnings {
				if w.Type == models.WarningTypeNoJoinPredicate {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Missing Join Predicate - Cartesian Product",
				"Tables joined without predicate. Every row from table A combined with every row from table B. Row count explodes exponentially.",
				"Add appropriate ON clause or WHERE condition to join tables properly",
				"Exponential row explosion, potential timeout, massive tempdb usage",
				models.SeverityCritical,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 7: No Join Predicate Warning",
		Tags:        []string{"join", "predicate", "cartesian", "warning"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "OrConditionNoIndex",
		Description: "OR condition may prevent index usage",
		Category:    "Predicate",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypePredicate,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.Predicate == nil {
				return false
			}
			return strings.Contains(op.Predicate.ScalarOperator, " OR ") && op.IndexScan != nil
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			tbl := getTableName(op)
			return makeBaseFinding(op,
				fmt.Sprintf("OR Condition on %s May Limit Index Usage", tbl),
				"OR conditions in WHERE clause often prevent efficient index usage. Multiple ORs may cause full scan.",
				"Consider UNION for different index paths, or ensure OR references same index",
				"May require multiple index scans or full table scan",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 7: OR and AND Conditions",
		Tags:        []string{"or", "condition", "index"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "NotEqualPredicate",
		Description: "NOT EQUAL predicate may limit index usage",
		Category:    "Predicate",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypePredicate,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.Predicate == nil {
				return false
			}
			ops := []string{"<>", "!=", "NOT "}
			for _, opStr := range ops {
				if strings.Contains(op.Predicate.ScalarOperator, opStr) {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"NOT EQUAL Predicate May Limit Index Usage",
				"<> or != operators typically cannot use index seek (only range seek for <= or >=). Most rows must be evaluated.",
				"Consider restructuring - if most rows are excluded, index may still help. Otherwise expect scan.",
				"May result in full scan as most data is excluded",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 7: Range Predicates",
		Tags:        []string{"not-equal", "predicate", "index"},
	})
}
