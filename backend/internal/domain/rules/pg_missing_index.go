package rules

import "github.com/rsharma155/sql_optima/internal/domain/evaluation"

func EvaluateMissingIndex(ctx evaluation.Context) evaluation.Result {
	seqScan := ctx["seq_scan_pct"]

	if seqScan > 70 {
		return evaluation.Result{
			RuleID:         "pg_missing_index",
			Severity:       "MEDIUM",
			Confidence:     0.6,
			Message:        "High sequential scan ratio → possible missing indexes",
			Recommendation: "Analyze queries for potential index candidates",
		}
	}

	return evaluation.Result{RuleID: "pg_missing_index", Severity: "OK", Confidence: 1.0}
}
