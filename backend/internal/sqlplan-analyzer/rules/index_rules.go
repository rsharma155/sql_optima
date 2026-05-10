// File: internal/sqlplan_rules/index_rules.go
// Purpose: Index-related rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerIndexRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "MissingIndexRecommendation",
		Description: "Missing index suggested by optimizer",
		Category:    "Indexing",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeIndex,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			for _, mi := range plan.MissingIndexes {
				if mi.Score > 0 {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			if len(plan.MissingIndexes) == 0 {
				return models.Finding{}
			}
			mi := plan.MissingIndexes[0]
			return makeBaseFinding(op,
				fmt.Sprintf("Missing Index on %s.%s", mi.Database, mi.Table),
				fmt.Sprintf("Optimizer suggested index on %s(%s). Impact score: %d", mi.Table, formatMissingIndexColumns(mi), mi.Score),
				fmt.Sprintf("CREATE INDEX %s ON %s(%s)", generateIndexName(mi), mi.Table, formatMissingIndexColumns(mi)),
				"Improved seek operations, reduced IO, faster query execution",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 9: Missing Index",
		Tags:        []string{"missing", "index", "recommendation"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "MissingIndexHighImpact",
		Description: "High impact missing index recommendation",
		Category:    "Indexing",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeIndex,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			for _, mi := range plan.MissingIndexes {
				if mi.Score >= 50 {
					return true
				}
			}
			return false
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			var highestScore int
			var bestMI *models.MissingIndex
			for i := range plan.MissingIndexes {
				if plan.MissingIndexes[i].Score > highestScore {
					highestScore = plan.MissingIndexes[i].Score
					bestMI = &plan.MissingIndexes[i]
				}
			}
			if bestMI == nil {
				return models.Finding{}
			}
			return makeBaseFinding(op,
				fmt.Sprintf("High Impact Missing Index on %s", bestMI.Table),
				fmt.Sprintf("Missing index has impact score of %d. Creating this index would significantly improve query performance.", highestScore),
				fmt.Sprintf("CREATE INDEX IX_%s ON %s(%s) INCLUDE (%s)", bestMI.Table, bestMI.Table, formatMissingIndexColumns(*bestMI), formatIncludedColumns(*bestMI)),
				"Major performance improvement expected",
				models.SeverityHigh,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 9: Missing Index",
		Tags:        []string{"missing", "index", "high-impact"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "ColumnNotInIndex",
		Description: "Required column not in index",
		Category:    "Indexing",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeIndex,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return op.IndexScan != nil && len(op.OutputList) > 0
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			tbl := getTableName(op)
			idx := getIndexName(op)
			seen := make(map[string]bool)
			missingCols := make([]string, 0)
			for _, col := range op.OutputList {
				if !seen[col.Column] {
					seen[col.Column] = true
					missingCols = append(missingCols, col.Column)
				}
			}
			return makeBaseFinding(op,
				fmt.Sprintf("Columns Not Covered by Index %s", idx),
				fmt.Sprintf("Query needs columns %s not in index. This causes bookmark lookup.", formatColumnList(missingCols)),
				fmt.Sprintf("Create covering index: CREATE INDEX ON %s(key) INCLUDE (%s)", tbl, formatColumnList(missingCols)),
				"Bookmark lookup required for each row, high IO",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 9: Covering Index",
		Tags:        []string{"covering", "include", "index"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "BookmarkLookupMultiple",
		Description: "Multiple bookmark lookups in plan",
		Category:    "Indexing",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			count := 0
			for _, o := range plan.Operators {
				if strings.Contains(o.PhysicalOp, "Key Lookup") || strings.Contains(o.PhysicalOp, "RID Lookup") {
					count++
				}
			}
			return count > 1
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			lookupCount := 0
			for _, o := range plan.Operators {
				if strings.Contains(o.PhysicalOp, "Key Lookup") || strings.Contains(o.PhysicalOp, "RID Lookup") {
					lookupCount++
				}
			}
			return makeBaseFinding(op,
				fmt.Sprintf("Multiple Bookmark Lookups (%d total)", lookupCount),
				"Plan contains multiple bookmark lookups. Each requires additional IO operation.",
				"Create covering indexes or consolidate into fewer indexes",
				"Multiple IO operations per row, high cumulative cost",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 3: Key Lookup",
		Tags:        []string{"bookmark", "lookup", "multiple"},
	})
}

func formatMissingIndexColumns(mi models.MissingIndex) string {
	cols := make([]string, len(mi.Columns))
	for i, col := range mi.Columns {
		cols[i] = col.Column
	}
	return strings.Join(cols, ", ")
}

func formatIncludedColumns(mi models.MissingIndex) string {
	if len(mi.IncludedColumns) == 0 {
		return ""
	}
	return strings.Join(mi.IncludedColumns, ", ")
}

func formatColumnList(cols []string) string {
	return strings.Join(cols, ", ")
}

func generateIndexName(mi models.MissingIndex) string {
	return fmt.Sprintf("IX_%s_%s", mi.Table, strings.Join(mi.IncludedColumns, "_"))
}
