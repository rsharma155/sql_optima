// File: internal/rules/advanced_rules.go
// Purpose: Advanced anti-pattern detection rules (parameter sniffing, joins, UDFs)
// Package: github.com/rsharma155/sqlplan-analyzer/internal/rules
package rules

import (
	"fmt"
	"strings"

	"github.com/rsharma155/sqlplan-analyzer/models"
)

type ParameterSniffingRule struct {
	BaseRule
}

func NewParameterSniffingRule() *ParameterSniffingRule {
	rule := &ParameterSniffingRule{}
	rule.name = "ParameterSniffingDetection"
	rule.description = "Detects parameter sniffing indicators"
	rule.ruleType = RuleTypeEstimation
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *ParameterSniffingRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, op := range plan.Operators {
		if op.EstimateRebinds > 0 || op.EstimateRewinds > 0 {
			finding := models.Finding{
				FindingType:     "ParameterSniffing",
				Severity:       models.SeverityMedium,
				OperatorID:    op.ID,
				OperatorName:  op.PhysicalOp,
				Title:        "Parameter sniffing detected: " + op.PhysicalOp,
				TechnicalExplanation: "Query was recompiled " + formatRebindsRewinds(op.EstimateRebinds, op.EstimateRewinds) + " times - indicates parameter sensitivity",
				BusinessExplanation: "The query optimizer made different plans for different parameter values, suggesting plan instability",
				Recommendation: "Consider OPTION (OPTIMIZE FOR UNKNOWN) or plan guides",
				Impact: "Variable performance based on parameter values",
				Confidence:  0.75,
				RuleName:   r.Name(),
				Category:  "Estimation",
			}
			findings = append(findings, finding)
		}
	}

	return findings
}

func formatRebindsRewinds(rebinds, rewinds int64) string {
	if rebinds > 0 && rewinds > 0 {
		return "rebinds and rewinds"
	} else if rebinds > 0 {
		return "rebinds"
	} else {
		return "rewinds"
	}
}

type BadNestedLoopRule struct {
	BaseRule
}

func NewBadNestedLoopRule() *BadNestedLoopRule {
	rule := &BadNestedLoopRule{}
	rule.name = "BadNestedLoopJoinDetection"
	rule.description = "Detects inefficient nested loop joins"
	rule.ruleType = RuleTypeJoin
	rule.severity = models.SeverityHigh
	rule.enabled = true
	return rule
}

func (r *BadNestedLoopRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	for _, op := range plan.Operators {
		if isNestedLoop(op) {
			for _, child := range op.Children {
				if child != nil && child.EstimatedTotalSubtreeCost > 1.0 {
					finding := models.Finding{
						FindingType:          "BadNestedLoopJoin",
						Severity:            models.SeverityHigh,
						OperatorID:          op.ID,
						OperatorName:        "Nested Loops",
						Title:               "Expensive nested loop join detected",
						TechnicalExplanation: "Nested loop with high estimated cost (" + formatCost(child.EstimatedTotalSubtreeCost) + ") in inner side",
						BusinessExplanation: "The query is repeatedly scanning a large table for each row from the outer table",
						Recommendation: "Consider hash or merge join instead for better performance",
						Impact: "High IO for large datasets",
						Confidence: 0.80,
						RuleName: r.Name(),
						Category: "Join",
					}
					findings = append(findings, finding)
					break
				}
			}
		}
	}

	return findings
}

func isNestedLoop(op models.Operator) bool {
	return strings.Contains(op.PhysicalOp, "Nested Loops") || strings.Contains(op.LogicalOp, "Nested Loops")
}

func formatCost(cost float64) string {
	return fmt.Sprintf("%.2f", cost)
}

func formatSpill(spills int64) string {
	if spills > 1000000 {
		return fmt.Sprintf("%.1fM", float64(spills)/1000000)
	}
	if spills > 1000 {
		return fmt.Sprintf("%.1fK", float64(spills)/1000)
	}
	return fmt.Sprintf("%d", spills)
}

type NonSargableRule struct {
	BaseRule
}

func NewNonSargableRule() *NonSargableRule {
	rule := &NonSargableRule{}
	rule.name = "NonSargableDetection"
	rule.description = "Detects non-SARGable predicates"
	rule.ruleType = RuleTypePredicate
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *NonSargableRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	nonSargablePatterns := []string{
		"LIKE '%",
		"NOT LIKE",
		"PATINDEX",
		"DATALENGTH",
		"LEN(",
	}

	for _, op := range plan.Operators {
		if op.Predicate != nil {
			predicate := op.Predicate.ScalarOperator
			for _, pattern := range nonSargablePatterns {
				if strings.Contains(predicate, pattern) && pattern == "LIKE '%" {
					finding := models.Finding{
						FindingType:          "NonSargablePredicate",
						Severity:            models.SeverityMedium,
						OperatorID:          op.ID,
						OperatorName:        op.PhysicalOp,
						Title:               "Non-SARGable predicate detected",
						TechnicalExplanation: "LIKE with leading wildcard prevents index seek",
						BusinessExplanation: "The query cannot use indexes efficiently due to pattern matching",
						Recommendation: "Rewrite using range queries or full-text search",
						Impact: "Full index/table scan required",
						Confidence: 0.85,
						RuleName: r.Name(),
						Category: "Predicate",
					}
					findings = append(findings, finding)
				}
			}
		}
	}

	return findings
}

type ScalarUDFRule struct {
	BaseRule
}

func NewScalarUDFRule() *ScalarUDFRule {
	rule := &ScalarUDFRule{}
	rule.name = "ScalarUDFDetection"
	rule.description = "Detects expensive scalar UDF usage"
	rule.ruleType = RuleTypeCompute
	rule.severity = models.SeverityMedium
	rule.enabled = true
	return rule
}

func (r *ScalarUDFRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	udfPatterns := []string{
		" scalar ",
		"dbo.",
		".*",
	}

	for _, op := range plan.Operators {
		for _, pattern := range udfPatterns {
			if strings.Contains(op.PhysicalOp, pattern) && strings.Contains(op.Action, "Function") {
				finding := models.Finding{
					FindingType:          "ScalarUDF",
					Severity:            models.SeverityMedium,
					OperatorID:          op.ID,
					OperatorName:        op.PhysicalOp,
					Title:               "Scalar function call detected: " + op.PhysicalOp,
					TechnicalExplanation: "Scalar UDFs are called per-row and prevent parallelism",
					BusinessExplanation: "Custom functions slow down the query significantly",
					Recommendation: "Consider rewriting as inline T-SQL or use inline table-valued functions",
					Impact: "Per-row execution, potential parallelism issues",
					Confidence: 0.75,
					RuleName: r.Name(),
					Category: "Compute",
				}
				findings = append(findings, finding)
				break
			}
		}
	}

	return findings
}

type HashSpillRule struct {
	BaseRule
}

func NewHashSpillRule() *HashSpillRule {
	rule := &HashSpillRule{}
	rule.name = "HashSpillDetection"
	rule.description = "Detects hash operation spills to tempdb"
	rule.ruleType = RuleTypeTempDB
	rule.severity = models.SeverityHigh
	rule.enabled = true
	return rule
}

func (r *HashSpillRule) Evaluate(plan *models.PlanAnalysis) []models.Finding {
	findings := make([]models.Finding, 0)

	hashOps := []string{"Hash Match", "Hash Aggregate"}

	for _, op := range plan.Operators {
		for _, hashOp := range hashOps {
			if strings.Contains(op.PhysicalOp, hashOp) && op.ActualSpills > 0 {
				finding := models.Finding{
					FindingType:          "HashSpill",
					Severity:            models.SeverityHigh,
					OperatorID:          op.ID,
					OperatorName:        op.PhysicalOp,
					Title:              "Hash operation spilled to tempdb",
					TechnicalExplanation: "Hash operation spilled " + formatSpill(op.ActualSpills) + " pages to tempdb",
					BusinessExplanation: "SQL Server ran out of memory and had to use disk storage",
					Recommendation: "Increase memory grant or reduce data size",
					Impact: "Significant disk I/O degradation",
					Confidence: 0.90,
					RuleName: r.Name(),
					Category: "Memory",
				}
				findings = append(findings, finding)
				break
			}
		}
	}

	return findings
}
