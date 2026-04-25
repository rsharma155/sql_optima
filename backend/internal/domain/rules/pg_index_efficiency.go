package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateIndexUsage(ctx evaluation.Context) evaluation.Result {
	scans := ctx["idx_scan"]
	size := ctx["index_size_mb"]

	if scans == 0 && size > 100 {
		return evaluation.Result{
			RuleID:         "pg_unused_index",
			Severity:       "HIGH",
			Confidence:     0.9,
			Message:        "Large index never used",
			Recommendation: "Consider dropping unused index",
		}
	}

	if scans < 10 && size > 50 {
		return evaluation.Result{
			RuleID:     "pg_unused_index",
			Severity:   "MEDIUM",
			Confidence: 0.7,
			Message:    "Index rarely used",
		}
	}

	return evaluation.Result{RuleID: "pg_unused_index", Severity: "OK", Confidence: 1.0}
}
