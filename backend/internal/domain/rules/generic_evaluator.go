package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func GenericThresholdEvaluator(ctx evaluation.Context, meta RuleMeta) evaluation.Result {
	value := ctx[meta.RuleID]
	threshold := meta.Threshold["value"]

	switch meta.ComparisonType {
	case "greater_than":
		if value > threshold {
			return evaluation.Result{
				RuleID:     meta.RuleID,
				Severity:   "HIGH",
				Confidence: 0.85,
				Message:    "Value exceeds threshold",
			}
		}
	case "less_than":
		if value < threshold {
			return evaluation.Result{
				RuleID:     meta.RuleID,
				Severity:   "HIGH",
				Confidence: 0.85,
				Message:    "Value below threshold",
			}
		}
	}

	return evaluation.Result{RuleID: meta.RuleID, Severity: "OK", Confidence: 1.0}
}
