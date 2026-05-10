// File: internal/sqlplan_rules/access_rules.go
// Purpose: Access method rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerAccessMethodRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "TableScanDetection",
		Description: "Full table scan without index",
		Category:    "AccessMethods",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return op.TableScan != nil
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			tbl := getTableName(op)
			return makeBaseFinding(op,
				fmt.Sprintf("Full Table Scan on %s", tbl),
				fmt.Sprintf("Table scan reads all %d rows from %s, requiring full data file read. This is one of the most expensive operations in SQL Server.", op.EstimateRows, tbl),
				fmt.Sprintf("Create an appropriate index on columns used in WHERE, JOIN, or ORDER BY clauses for table %s", tbl),
				"High IO cost, poor performance on large tables, blocks other queries",
				models.SeverityHigh,
			)
		},
		Priority:    1,
		BookChapter: "Chapter 2: Table Scan",
		Tags:        []string{"scan", "table", "io"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "ClusteredIndexScanDetection",
		Description: "Clustered index scan detected",
		Category:    "AccessMethods",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.IndexScan == nil {
				return false
			}
			return op.IndexScan.IndexKind == "ClusteredIndex" && !strings.Contains(op.PhysicalOp, "Seek")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			tbl := getTableName(op)
			idx := getIndexName(op)
			return makeBaseFinding(op,
				fmt.Sprintf("Clustered Index Scan on %s(%s)", tbl, idx),
				fmt.Sprintf("Clustered index scan on %s reads all leaf pages. Estimated %d rows.", tbl, op.EstimateRows),
				"Consider index on filtered columns or covering index to enable seek operation",
				"High IO for large tables, consider covering index",
				models.SeverityMedium,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 2: Clustered Index Scan",
		Tags:        []string{"clustered", "scan", "index"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "IndexScanInsteadOfSeek",
		Description: "Index scan used instead of seek",
		Category:    "AccessMethods",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.IndexScan == nil {
				return false
			}
			return !strings.Contains(op.PhysicalOp, "Seek") && !strings.Contains(op.PhysicalOp, "Table Scan")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			tbl := getTableName(op)
			idx := getIndexName(op)
			return makeBaseFinding(op,
				fmt.Sprintf("Index Scan on %s(%s) instead of Seek", tbl, idx),
				"Optimizer chose index scan even when seek would be more efficient. This suggests predicates may not align with index key.",
				"Review WHERE clause alignment with index key columns. Ensure leading column of index is used in predicates.",
				"Unnecessary IO operations, all index pages read",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 2: Scans vs Seeks",
		Tags:        []string{"scan", "seek", "index"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "KeyLookupDetection",
		Description: "Key lookup operation detected",
		Category:    "AccessMethods",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Key Lookup")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Key Lookup (Bookmark Lookup) Operation",
				"Bookmark lookup required to fetch columns not in the index. For each row returned by index seek, an additional lookup is performed to get remaining columns.",
				"Create a covering index with INCLUDE columns for all columns needed by the query",
				"Performance degradation proportional to number of lookups, additional IO per row",
				models.SeverityHigh,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 3: Key Lookup",
		Tags:        []string{"keylookup", "bookmark", "covering"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "RIDLookupDetection",
		Description: "RID lookup operation detected",
		Category:    "AccessMethods",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "RID Lookup")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"RID Lookup on Heap",
				"RID lookup uses row identifier to fetch rows from heap table. Similar to Key Lookup but for tables without clustered index.",
				"Create a clustered index on the table to enable Key Lookup instead of RID Lookup",
				"High IO operations, fragmented heap access",
				models.SeverityHigh,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 3: RID Lookup",
		Tags:        []string{"ridlookup", "heap", "bookmark"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "NonClusteredIndexScanDetection",
		Description: "Non-clustered index scan detected",
		Category:    "AccessMethods",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeAccessPath,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if op.IndexScan == nil {
				return false
			}
			return op.IndexScan.IndexKind == "NonClustered" && !strings.Contains(op.PhysicalOp, "Seek")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			tbl := getTableName(op)
			idx := getIndexName(op)
			return makeBaseFinding(op,
				fmt.Sprintf("Non-Clustered Index Scan on %s(%s)", tbl, idx),
				"Full non-clustered index scan reads all index pages.",
				"Consider seek operation or more selective index",
				"IO cost proportional to index size",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 2: Non-clustered Index Scan",
		Tags:        []string{"nc-index", "scan"},
	})
}
