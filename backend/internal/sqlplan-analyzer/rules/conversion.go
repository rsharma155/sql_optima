// File: internal/rules/conversion.go
// Purpose: Implicit conversion and predicate anti-pattern detection rules
// Package: github.com/rsharma155/sqlplan-analyzer/internal/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type ImplicitConversionRule struct {
	BaseRule
}

func NewImplicitConversionRule() *ImplicitConversionRule {
	rule := &ImplicitConversionRule{}
	rule.name = "ImplicitConversionDetection"
	rule.description = "Detects implicit type conversions that suppress index usage"
	rule.ruleType = RuleTypePredicate
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *ImplicitConversionRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, warning := range plan.Warnings {
		if warning.Type == models.WarningTypeTypeConversion {
			finding := models.Finding{
				FindingType:          "ImplicitConversion",
				Severity:            models.SeverityMedium,
				Title:               "Implicit type conversion detected in predicate",
				TechnicalExplanation: warning.Message + " - This can cause index suppression as SQL Server cannot use indexes on converted columns.",
				BusinessExplanation:  "The query uses mismatched data types, forcing SQL Server to convert values on-the-fly, which prevents using indexes for fast lookups.",
				Recommendation:          "Ensure column and parameter data types match exactly",
				Impact:                "Index usage suppressed, potential full scans",
				Confidence:            0.85,
				RuleName:              r.Name(),
				RuleEnabled:           r.Enabled(),
				Category:             "Predicate",
				SubCategory:          "Implicit Conversion",
			}
			findings = append(findings, finding)
		}
	}

	for _, op := range plan.Operators {
		if hasImplicitConversion(op) {
			finding := models.Finding{
				FindingType:          "ImplicitConversion",
				Severity:            models.SeverityMedium,
				OperatorID:          op.ID,
				OperatorName:        op.PhysicalOp,
				Title:               "Function or conversion on indexed column: " + op.PhysicalOp,
				TechnicalExplanation: "A function or implicit conversion was applied to an indexed column, preventing index seek.",
				BusinessExplanation:  "SQL Server had to evaluate every row because the indexed column was modified by a function.",
				Recommendation:          "Rewrite query to avoid functions on indexed columns in WHERE clause",
				Impact:                "Index seek suppressed",
				Confidence:            0.80,
				EstimatedCost:        op.EstimatedTotalSubtreeCost,
				RuleName:              r.Name(),
				RuleEnabled:           r.Enabled(),
				Category:             "Predicate",
				SubCategory:          "Function on Index",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

func hasImplicitConversion(op models.Operator) bool {
	if op.Predicate != nil && len(op.Predicate.ScalarOperator) > 0 {
		convFunctions := []string{"CONVERT", "CAST", "YEAR", "MONTH", "DAY", "LEN", "DATALENGTH", "SUBSTRING", "RTRIM", "LTRIM"}
		for _, fn := range convFunctions {
			if strings.Contains(op.Predicate.ScalarOperator, fn) {
				return true
			}
		}
	}
	return false
}

type SpillRule struct {
	BaseRule
}

func NewSpillRule() *SpillRule {
	rule := &SpillRule{}
	rule.name = "SpillToTempDBDetection"
	rule.description = "Detects sort and hash spills to tempdb"
	rule.ruleType = RuleTypeTempDB
	rule.severity = models.SeverityHigh
	rule.enabled = true
	return rule
}

func (r *SpillRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, warning := range plan.Warnings {
		if warning.Type == models.WarningTypeSpillToTempDB {
			finding := models.Finding{
				FindingType:           "SpillToTempDB",
				Severity:             models.SeverityHigh,
				Title:                "Temporary data spill to tempdb",
				TechnicalExplanation: "Memory was insufficient, causing data to be written to tempdb causing disk I/O.",
				BusinessExplanation:  "SQL Server ran out of memory and had to use the disk as scratch space, like running out of desk space and spreading papers on the floor.",
				Recommendation:          "Increase memory grant or optimize query to reduce data size",
				Impact:                "Significant disk I/O, degraded performance",
				Confidence:            0.95,
				RuleName:              r.Name(),
				RuleEnabled:           r.Enabled(),
				Category:            "Memory",
				SubCategory:         "TempDB Spill",
			}
			findings = append(findings, finding)
		}
	}

	for _, op := range plan.Operators {
		if isSortOrHashOperator(op) && op.ActualSpills > 0 {
			finding := models.Finding{
				FindingType:           "SpillToTempDB",
				Severity:             models.SeverityHigh,
				OperatorID:           op.ID,
				OperatorName:         op.PhysicalOp,
				Title:                "Operation spilled to tempdb: " + op.PhysicalOp,
				TechnicalExplanation: "The " + op.PhysicalOp + " operation spilled " + formatSpillCount(op.ActualSpills) + " pages to tempdb.",
				BusinessExplanation:  "This operation ran out of memory and had to use temporary disc storage.",
				Recommendation:          "Increase memory grant or optimize the operation",
				Impact:                "Disk I/O, slow performance",
				Confidence:            0.90,
				NumericImpact:       float64(op.ActualSpills),
				EstimatedCost:      op.EstimatedTotalSubtreeCost,
				RuleName:            r.Name(),
				RuleEnabled:         r.Enabled(),
				Category:           "Memory",
				SubCategory:        "Spill",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

func isSortOrHashOperator(op models.Operator) bool {
	return strings.Contains(op.PhysicalOp, "Sort") ||
		strings.Contains(op.PhysicalOp, "Hash") ||
		strings.Contains(op.PhysicalOp, "Aggregate")
}

func formatSpillCount(count int64) string {
	if count > 1000000 {
		return fmt.Sprintf("%dM", count/1000000)
	}
	if count > 1000 {
		return fmt.Sprintf("%dK", count/1000)
	}
	return fmt.Sprintf("%d", count)
}

type MissingIndexRule struct {
	BaseRule
}

func NewMissingIndexRule() *MissingIndexRule {
	rule := &MissingIndexRule{}
	rule.name = "MissingIndexDetection"
	rule.description = "Analyzes missing index recommendations"
	rule.ruleType = RuleTypeIndex
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *MissingIndexRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, mi := range plan.MissingIndexes {
		_ = generateCreateIndexSQL(mi)

		finding := models.Finding{
			FindingType:           "MissingIndex",
			Severity:            models.SeverityMedium,
			Title:                "Missing index on " + mi.Table,
			TechnicalExplanation: "The query optimizer suggests creating an index to improve performance.",
			BusinessExplanation:  "The database is missing an index that would help find data faster.",
			Recommendation:        "Consider creating the suggested index after evaluating impact",
			Impact:               "Potential improvement depends on query workload",
			Confidence:          0.70,
			RuleName:            r.Name(),
			RuleEnabled:         r.Enabled(),
			Category:           "Index",
			SubCategory:        "Missing Index",
		}
		findings = append(findings, finding)
	}

	return findings
}

func generateCreateIndexSQL(mi models.MissingIndex) string {
	var sb strings.Builder

	sb.WriteString("CREATE NONCLUSTERED INDEX IX_")
	sb.WriteString(mi.Table)
	sb.WriteString("_")
	sb.WriteString(mi.Schema)

	for i, col := range mi.Columns {
		if i > 0 {
			sb.WriteString("_")
		}
		sb.WriteString(col.Column)
	}
	sb.WriteString(" ON ")
	sb.WriteString(mi.Database)
	sb.WriteString(".")
	sb.WriteString(mi.Schema)
	sb.WriteString(".")
	sb.WriteString(mi.Table)
	sb.WriteString(" (")

	for i, col := range mi.Columns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(col.Column)
	}
	sb.WriteString(")")

	if len(mi.IncludedColumns) > 0 {
		sb.WriteString(" INCLUDE (")
		for i, col := range mi.IncludedColumns {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(col)
		}
		sb.WriteString(")")
	}

	sb.WriteString(";")

	return sb.String()
}
