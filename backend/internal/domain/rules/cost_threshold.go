package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateCostThreshold(ctx evaluation.Context) evaluation.Result {
	cost := ctx["cost_threshold"]
	parallelUsage := ctx["parallel_usage_pct"]

	if cost < 20 && parallelUsage > 30 {
		return evaluation.Result{
			RuleID:      "cost_threshold",
			Severity:    "HIGH",
			Confidence: 0.85,
			Message:     "Low cost threshold causing excessive parallelism",
		}
	}

	if cost < 20 {
		return evaluation.Result{
			RuleID:      "cost_threshold",
			Severity:    "MEDIUM",
			Confidence: 0.7,
		}
	}

	return evaluation.Result{RuleID: "cost_threshold", Severity: "OK", Confidence: 1.0}
}