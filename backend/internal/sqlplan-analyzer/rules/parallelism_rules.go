// File: internal/sqlplan_rules/parallelism_rules.go
// Purpose: Parallelism rules based on SQL Server Execution Plans book
// Package: github.com/rsharma155/sqlplan-analyzer/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

func (e *RuleEngine) registerParallelismRules() {
	e.rules = append(e.rules, RuleDefinition{
		Name:        "ParallelismSkew",
		Description: "Uneven thread distribution detected",
		Category:    "Parallelism",
		Severity:    models.SeverityHigh,
		RuleType:    RuleTypeParallelism,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if plan.QueryPlan == nil {
				return false
			}
			dop := plan.QueryPlan.DegreeOfParallelism
			if dop <= 1 {
				return false
			}
			return calculateThreadSkew(op) > 2.0
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			dop := 1
			if plan.QueryPlan != nil {
				dop = plan.QueryPlan.DegreeOfParallelism
			}
			return makeBaseFinding(op,
				"Thread Skew in Parallel Operation",
				fmt.Sprintf("Parallel threads have uneven work distribution with DOP %d. Some threads complete faster while others continue processing.", dop),
				"Review join types, large sorts, or data skew issues. Consider partitioned tables or filtered indexes.",
				"Overall execution time determined by slowest thread",
				models.SeverityHigh,
			)
		},
		Priority:    2,
		BookChapter: "Chapter 6: Parallelism",
		Tags:        []string{"parallel", "thread", "skew"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "ExcessiveDOP",
		Description: "Degree of parallelism too high",
		Category:    "Parallelism",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeParallelism,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if plan.QueryPlan == nil || op.ID != 1 {
				return false
			}
			return plan.QueryPlan.DegreeOfParallelism > 8 && op.EstimateRows < 10000
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Excessive DOP for Small Result Set",
				fmt.Sprintf("DOP of %d for only %.0f estimated rows. Thread creation overhead exceeds parallel benefit.", plan.QueryPlan.DegreeOfParallelism, op.EstimateRows),
				"Consider OPTION (MAXDOP 1) for small result sets or set server MAXDOP to lower value",
				"Context switching overhead, memory pressure",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 6: MAXDOP",
		Tags:        []string{"dop", "parallel", "excessive"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "NoParallelismAvailable",
		Description: "Serial execution for complex query",
		Category:    "Parallelism",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeParallelism,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			if plan.QueryPlan == nil {
				return false
			}
			return plan.QueryPlan.DegreeOfParallelism <= 1 &&
				len(plan.Operators) > 10 &&
				op.EstimatedTotalSubtreeCost > 5.0
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Serial Execution for Complex Query",
				"Complex query with many operators running single-threaded. Could benefit from parallelism.",
				"Check MAXDOP setting, query hints, or resource governor configuration",
				"Long execution time for CPU-intensive query",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 6: Parallelism",
		Tags:        []string{"serial", "no-parallel"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "GatherStreamsOverhead",
		Description: "Gather streams operation overhead",
		Category:    "Parallelism",
		Severity:    models.SeverityMedium,
		RuleType:    RuleTypeParallelism,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Gather Streams")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Gather Streams",
				"Parallel threads converging to single thread. Data must be merged and sorted.",
				"Ensure benefit of parallel execution outweighs gather overhead. Consider if query needs all rows at root.",
				"Thread synchronization overhead",
				models.SeverityMedium,
			)
		},
		Priority:    3,
		BookChapter: "Chapter 6: Gather Streams",
		Tags:        []string{"gather", "parallel", "streams"},
	})

	e.rules = append(e.rules, RuleDefinition{
		Name:        "DistributeStreams",
		Description: "Distribute streams to parallel threads",
		Category:    "Parallelism",
		Severity:    models.SeverityLow,
		RuleType:    RuleTypeParallelism,
		Condition: func(plan *models.PlanAnalysis, op *models.Operator) bool {
			return strings.Contains(op.PhysicalOp, "Distribute Streams")
		},
		ExtractFinding: func(plan *models.PlanAnalysis, op *models.Operator) models.Finding {
			return makeBaseFinding(op,
				"Distribute Streams",
				"Data partitioned across parallel threads. Distribution method (hash, broadcast, round-robin) affects performance.",
				"Review distribution method and consider partitioning strategy",
				"Distribution overhead varies by method",
				models.SeverityLow,
			)
		},
		Priority:    4,
		BookChapter: "Chapter 6: Distribute Streams",
		Tags:        []string{"distribute", "parallel", "streams"},
	})
}

func calculateThreadSkew(op *models.Operator) float64 {
	if len(op.RuntimeCounters) < 2 {
		return 0
	}

	var totalRows int64
	var maxRows int64

	for _, rc := range op.RuntimeCounters {
		totalRows += rc.ActualRows
		if rc.ActualRows > maxRows {
			maxRows = rc.ActualRows
		}
	}

	if totalRows == 0 {
		return 0
	}

	avgRows := float64(totalRows) / float64(len(op.RuntimeCounters))
	if avgRows == 0 {
		return 0
	}

	return float64(maxRows) / avgRows
}

