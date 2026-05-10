// File: internal/rules/table_scan.go
// Purpose: Table scan anti-pattern detection rule
// Package: github.com/rsharma155/sqlplan-analyzer/internal/rules
package rules

import (
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type TableScanRule struct {
	BaseRule
}

func NewTableScanRule() *TableScanRule {
	rule := &TableScanRule{}
	rule.name = "TableScanDetection"
	rule.description = "Detects full table scans which indicate missing indexes"
	rule.ruleType = RuleTypeAccessPath
	rule.severity = models.SeverityHigh
	rule.enabled = true
	return rule
}

func (r *TableScanRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, op := range plan.Operators {
		if op.TableScan != nil {
			finding := models.Finding{
				FindingType:          "TableScan",
				Severity:            models.SeverityHigh,
				OperatorID:          op.ID,
				OperatorName:        "Table Scan",
				Title:               "Full table scan detected on " + op.TableScan.Object.Table,
				TechnicalExplanation: "The query performed a full table scan on " + op.TableScan.Object.Table + ". This means SQL Server had to read every row in the table.",
				BusinessExplanation:  "SQL Server had to read every single row in the table, similar to reading every page in a book to find a single word.",
				Recommendation:          "Create a covering index for the columns used in WHERE, JOIN, and ORDER BY clauses",
				Impact:                "High IO - reading all rows unnecessarily",
				Confidence:            0.95,
				EstimatedCost:        op.EstimatedTotalSubtreeCost,
				RuleName:              r.Name(),
				RuleEnabled:           r.Enabled(),
				Category:             "Access Path",
				SubCategory:          "Table Scan",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

type IndexScanRule struct {
	BaseRule
}

func NewIndexScanRule() *IndexScanRule {
	rule := &IndexScanRule{}
	rule.name = "IndexScanDetection"
	rule.description = "Detects index scans that could be seeks"
	rule.ruleType = RuleTypeAccessPath
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *IndexScanRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, op := range plan.Operators {
		if op.IndexScan != nil && isIndexScan(op) {
			scanTypeName := "Index Scan"
			if op.IndexScan.IndexKind == "Heap" {
				scanTypeName = "Heap Scan"
			} else if op.IndexScan.IndexKind == "Clustered" {
				scanTypeName = "Clustered Index Scan"
			}

			severity := models.SeverityMedium
			if op.EstimateRows > 10000 {
				severity = models.SeverityHigh
			}

			finding := models.Finding{
				FindingType:          "IndexScan",
				Severity:            severity,
				OperatorID:          op.ID,
				OperatorName:        scanTypeName,
				Title:               scanTypeName + " on " + op.IndexScan.Object.Table,
				TechnicalExplanation: "The query performed an index scan instead of a seek. Index scans must evaluate all index entries.",
				BusinessExplanation:  "Instead of jumping directly to the relevant data using the index, SQL Server had to examine every entry in the index.",
				Recommendation:          "Review WHERE clause predicates for SARGable conditions",
				Impact:                "Potential high IO on large tables",
				Confidence:            0.90,
				EstimatedCost:        op.EstimatedTotalSubtreeCost,
				RuleName:              r.Name(),
				RuleEnabled:           r.Enabled(),
				Category:             "Access Path",
				SubCategory:          "Index Scan",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

func isIndexScan(op models.Operator) bool {
	return strings.Contains(op.PhysicalOp, "Index Scan") ||
		strings.Contains(op.PhysicalOp, "Clustered Index Scan") ||
		strings.Contains(op.PhysicalOp, "Heap Scan")
}

type KeyLookupRule struct {
	BaseRule
}

func NewKeyLookupRule() *KeyLookupRule {
	rule := &KeyLookupRule{}
	rule.name = "KeyLookupDetection"
	rule.description = "Detects expensive key lookups and bookmark lookups"
	rule.ruleType = RuleTypeAccessPath
	rule.severity = models.SeverityHigh
	rule.enabled = true
	return rule
}

func (r *KeyLookupRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, op := range plan.Operators {
		if isKeyLookup(op) {
			finding := models.Finding{
				FindingType:          "KeyLookup",
				Severity:            models.SeverityHigh,
				OperatorID:          op.ID,
				OperatorName:        "Key Lookup",
				Title:               "Expensive key lookup detected on " + op.PhysicalOp,
				TechnicalExplanation: "A key lookup (bookmark lookup) was performed for each row. This causes random I/O operations.",
				BusinessExplanation:  "SQL Server had to go back to the storage repeatedly for each row, like having to walk across a warehouse to get items listed on your shopping list.",
				Recommendation:          "Add INCLUDE columns to the covering index to cover all required columns",
				Impact:                "High random I/O - millions of page reads for large result sets",
				Confidence:            0.92,
				EstimatedCost:        op.EstimatedTotalSubtreeCost,
				RuleName:              r.Name(),
				RuleEnabled:           r.Enabled(),
				Category:             "Access Path",
				SubCategory:          "Key Lookup",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

func isKeyLookup(op models.Operator) bool {
	return strings.Contains(op.PhysicalOp, "Key Lookup") ||
		strings.Contains(op.PhysicalOp, "RID Lookup") ||
		strings.Contains(op.PhysicalOp, "Clustered Index Seek")
}
