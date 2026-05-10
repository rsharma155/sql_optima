// File: internal/rules/parallelism.go
// Purpose: Parallelism skew and threading anti-pattern detection rules
// Package: github.com/rsharma155/sqlplan-analyzer/internal/rules
package rules

import (
	"fmt"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type ParallelismSkewRule struct {
	BaseRule
}

func NewParallelismSkewRule() *ParallelismSkewRule {
	rule := &ParallelismSkewRule{}
	rule.name = "ParallelismSkewDetection"
	rule.description = "Detects thread skew in parallel query execution"
	rule.ruleType = RuleTypeParallelism
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *ParallelismSkewRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	if plan.QueryPlan == nil || plan.QueryPlan.DegreeOfParallelism <= 1 {
		return findings
	}

	threadCounts := make([]int64, 0)
	var maxThreadRows, minThreadRows int64 = 0, ^int64(0)

	for _, op := range plan.Operators {
		if op.Parallel {
			threadCounts = append(threadCounts, op.ActualRows)
			if op.ActualRows > maxThreadRows {
				maxThreadRows = op.ActualRows
			}
			if op.ActualRows < minThreadRows {
				minThreadRows = op.ActualRows
			}
		}
	}

	if len(threadCounts) > 1 && maxThreadRows > 0 {
		skewRatio := float64(maxThreadRows) / float64(minThreadRows)
		if skewRatio > 10.0 {
			finding := models.Finding{
				FindingType:          "ParallelismSkew",
				Severity:            models.SeverityMedium,
				Title:               "Thread distribution skew detected",
				TechnicalExplanation: "Thread skew ratio: " + formatFloat(skewRatio) + ". Some threads processed significantly more rows than others.",
				BusinessExplanation:  "Some workers did much more work than others while waiting for the busiest one, like a team where one person does most of the work.",
				Recommendation:          "Review data distribution and consider filtered indexes or partition modifications",
				Impact:                "Underutilized parallelism, potential performance degradation",
				Confidence:            0.75,
				NumericImpact:        skewRatio,
				RuleName:              r.Name(),
				RuleEnabled:           r.Enabled(),
				Category:             "Parallelism",
				SubCategory:          "Thread Skew",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

type CardinalityEstimationRule struct {
	BaseRule
}

func NewCardinalityEstimationRule() *CardinalityEstimationRule {
	rule := &CardinalityEstimationRule{}
	rule.name = "CardinalityEstimationDetection"
	rule.description = "Detects cardinality estimation issues"
	rule.ruleType = RuleTypeEstimation
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *CardinalityEstimationRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	thresholdRatio := 100.0

	for _, op := range plan.Operators {
		if op.EstimateRows == 0 {
			continue
		}

		ratio := float64(op.ActualRows) / float64(op.EstimateRows)

		if ratio > thresholdRatio || ratio < 1.0/thresholdRatio {
			severity := models.SeverityMedium
			if ratio > thresholdRatio*10 || ratio < 1.0/(thresholdRatio*10) {
				severity = models.SeverityHigh
			}

			direction := "underestimated"
			if ratio > 1 {
				direction = "underestimated"
			}

			finding := models.Finding{
				FindingType:          "CardinalityEstimationIssue",
				Severity:            severity,
				OperatorID:          op.ID,
				OperatorName:        op.PhysicalOp,
				Title:               "Cardinality estimation " + direction + ": " + op.PhysicalOp,
				TechnicalExplanation: "Estimated " + formatInt(op.EstimateRows) + " rows, actual " + formatInt(op.ActualRows) + " rows (ratio: " + formatFloat(ratio) + "x)",
				BusinessExplanation:  "The query optimizer misjudged how much data would be processed by a factor of " + formatFloat(ratio) + " times.",
				Recommendation:      "Update statistics or consider query hints for better estimation",
				Impact:              "Suboptimal execution plan selection",
				Confidence:          0.80,
				NumericImpact:      ratio,
				EstimatedCost:      op.EstimatedTotalSubtreeCost,
				RuleName:            r.Name(),
				RuleEnabled:         r.Enabled(),
				Category:           "Estimation",
				SubCategory:        "Cardinality",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

func formatInt(i int64) string {
	return fmt.Sprintf("%d", i)
}
