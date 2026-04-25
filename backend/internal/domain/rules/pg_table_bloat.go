package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateTableBloat(ctx evaluation.Context) evaluation.Result {
	ratio := ctx["bloat_ratio"]

	if ratio > 1.5 {
		return evaluation.Result{
			RuleID:         "pg_table_bloat",
			Severity:       "HIGH",
			Confidence:     0.85,
			Message:        "Significant table bloat detected",
			Recommendation: "Consider VACUUM FULL or pg_repack",
		}
	}

	if ratio > 1.2 {
		return evaluation.Result{
			RuleID:     "pg_table_bloat",
			Severity:   "MEDIUM",
			Confidence: 0.7,
			Message:    "Moderate table bloat detected",
		}
	}

	return evaluation.Result{RuleID: "pg_table_bloat", Severity: "OK", Confidence: 1.0}
}
